package server

import (
	"encoding/json"
	"fmt"
	"github.com/pingcap/errors"
	"github.com/siddontang/go-log/log"
	"github.com/siddontang/go-mysql/canal"
	"github.com/siddontang/go-mysql/mysql"
	"github.com/siddontang/go-mysql/replication"
	"github.com/siddontang/go-mysql/schema"
	"porter/syncer"
	"reflect"
	"strings"
	"time"
)

const (
	fieldTypeList = "list"
	fieldTypeDate = "date"
)

const mysqlDateFormat = "2016-01-02"

type posSaver struct {
	pos   mysql.Position
	force bool
}

type eventHandler struct {
	s *Server
}

func (h *eventHandler) OnRotate(e *replication.RotateEvent) error {
	position := mysql.Position{
		Name: string(e.NextLogName),
		Pos:  uint32(e.Position),
	}
	h.s.syncCh <- posSaver{
		pos:   position,
		force: true,
	}
	return h.s.ctx.Err()
}

func (h *eventHandler) OnTableChanged(schema, table string) error {
	err := h.s.updateRule(schema, table)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (h *eventHandler) OnDDL(nextPos mysql.Position, _ *replication.QueryEvent) error {
	h.s.syncCh <- posSaver{
		pos:   nextPos,
		force: true,
	}
	return h.s.ctx.Err()
}

func (h *eventHandler) OnXID(nextPos mysql.Position) error {
	h.s.syncCh <- posSaver{
		pos:   nextPos,
		force: false,
	}
	return h.s.ctx.Err()
}

func (h *eventHandler) OnRow(e *canal.RowsEvent) error {
	rule, ok := h.s.rules[ruleKey(e.Table.Schema, e.Table.Name)]
	if !ok {
		fmt.Println(rule)
		return nil
	}

	var err error
	switch e.Action {
	case canal.InsertAction:
	case canal.DeleteAction:
	case canal.UpdateAction:
	default:
		err = errors.Errorf("invalid rows action %s", e.Action)
	}

	h.s.assembly(rule, e.Action, e.Rows)

	if err != nil {
		h.s.cancel()
		return errors.Errorf("OnRow %s err %v, close sync", e.Action, err)
	}

	h.s.syncCh <- e

	return h.s.ctx.Err()
}

// assemblyRows assembly data.
func (s *Server) assembly(rule *syncer.Rule, action string, rows [][]interface{}) error {
	datas := make([]*syncer.Processor, 0, len(rows))

	for _, values := range rows {
		pd, err := s.makeProcessorData(rule, values)
		if err != nil {
			s.cancel()
			return errors.Errorf("assembly data error, err:%s", err)
		}

		data := &syncer.Processor{
			Action: action,
			Schema: rule.Schema,
			Table:  rule.Table,
			Data:   pd,
		}

		datas = append(datas, data)
	}

	fmt.Printf("binlog : %s", datas)
	return nil
}

func (s *Server) makeProcessorData(rule *syncer.Rule, values []interface{}) (map[string]interface{}, error) {
	data := make(map[string]interface{}, len(values))

	for i, c := range rule.TableInfo.Columns {
		if !rule.CheckFilter(c.Name) {
			continue
		}
		mapped := false
		for k, v := range rule.FieldMapping {
			mysql, tMysql, fieldType := s.getFieldParts(k, v)
			if mysql == c.Name {
				mapped = true
				data[tMysql] = s.getFieldValue(&c, fieldType, values[i])
			}
		}
		if mapped == false {
			data[c.Name] = s.makeColumnData(&c, values[i])
		}
	}
	return data, nil
}

func (h *eventHandler) OnGTID(gtid mysql.GTIDSet) error {
	return nil
}

func (h *eventHandler) OnPosSynced(pos mysql.Position, set mysql.GTIDSet, force bool) error {
	return nil
}

func (h *eventHandler) String() string {
	return "PorterEventHandler"
}

func (s *Server) syncLoop() {
	// todu
}

func (s *Server) getFieldParts(k, v string) (string, string, string) {
	composedField := strings.Split(v, ",")

	mysql := k
	tMysql := composedField[0]
	fieldType := ""

	if 0 == len(tMysql) {
		tMysql = mysql
	}
	if 2 == len(composedField) {
		fieldType = composedField[1]
	}

	return mysql, tMysql, fieldType
}

// getFieldData get mysql field value and convert it to specific value to tMysql.
func (s *Server) getFieldValue(col *schema.TableColumn, fieldType string, value interface{}) interface{} {
	var fieldValue interface{}
	switch fieldType {
	case fieldTypeList:
		v := s.makeColumnData(col, value)
		if str, ok := v.(string); ok {
			fieldValue = strings.Split(str, ",")
		} else {
			fieldValue = v
		}
	case fieldTypeDate:
		if col.Type == schema.TYPE_NUMBER {
			col.Type = schema.TYPE_DATETIME

			v := reflect.ValueOf(value)
			switch v.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				fieldValue = s.makeColumnData(col, time.Unix(v.Int(), 0).Format(mysql.TimeFormat))
			}
		}
	}
	return fieldValue
}

func (s *Server) makeColumnData(col *schema.TableColumn, value interface{}) interface{} {
	switch col.Type {
	case schema.TYPE_ENUM:
		switch value := value.(type) {
		case int64:
			// for binlog, ENUM may be int64, but for dump, enum is string
			eNum := value - 1
			if eNum < 0 || eNum >= int64(len(col.EnumValues)) {
				// we insert invalid enum value before, so return empty
				log.Warnf("invalid binlog enum index %d, for enum %v", eNum, col.EnumValues)
				return ""
			}

			return col.EnumValues[eNum]
		}
	case schema.TYPE_SET:
		switch value := value.(type) {
		case int64:
			// for binlog, SET may be int64, but for dump, SET is string
			bitmask := value
			sets := make([]string, 0, len(col.SetValues))
			for i, s := range col.SetValues {
				if bitmask&int64(1<<uint(i)) > 0 {
					sets = append(sets, s)
				}
			}
			return strings.Join(sets, ",")
		}
	case schema.TYPE_BIT:
		switch value := value.(type) {
		case string:
			// for binlog, BIT is int64, but for dump, BIT is string
			// for dump 0x01 is for 1, \0 is for 0
			if value == "\x01" {
				return int64(1)
			}

			return int64(0)
		}
	case schema.TYPE_STRING:
		switch value := value.(type) {
		case []byte:
			return string(value[:])
		}
	case schema.TYPE_JSON:
		var f interface{}
		var err error
		switch v := value.(type) {
		case string:
			err = json.Unmarshal([]byte(v), &f)
		case []byte:
			err = json.Unmarshal(v, &f)
		}
		if err == nil && f != nil {
			return f
		}
	case schema.TYPE_DATETIME, schema.TYPE_TIMESTAMP:
		switch v := value.(type) {
		case string:
			vt, err := time.ParseInLocation(mysql.TimeFormat, string(v), time.Local)
			if err != nil || vt.IsZero() { // failed to parse date or zero date
				return nil
			}
			return vt.Format(time.RFC3339)
		}
	case schema.TYPE_DATE:
		switch v := value.(type) {
		case string:
			vt, err := time.Parse(mysqlDateFormat, string(v))
			if err != nil || vt.IsZero() { // failed to parse date or zero date
				return nil
			}
			return vt.Format(mysqlDateFormat)
		}
	}

	return value
}
