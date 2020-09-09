package server

import (
	"encoding/json"
	"fmt"
	"github.com/coreos/etcd/pkg/types"
	"github.com/pingcap/errors"
	"github.com/siddontang/go-mysql/canal"
	"github.com/siddontang/go-mysql/mysql"
	"github.com/siddontang/go-mysql/replication"
	"github.com/siddontang/go-mysql/schema"
	"porter/config"
	"porter/log"
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

type SyncerType int8

const (
	PREPARE SyncerType = iota
	START
	RUNNING
	STOP
	UPDATE
)

func (st SyncerType) String() string {
	types := []string{
		"PREPARE",
		"START",
		"RUNNING",
		"STOP",
		"UPDATE",
	}
	return types[int(st)]
}

type BinlogSyncer struct {
	*Server
}

func NewBinlogSyncer(srv *Server) *BinlogSyncer {
	return &BinlogSyncer{srv}
}

// Leader return leader id
func (b *BinlogSyncer) Leader() types.ID {
	lead := b.config.RaftNodeConfig.Node.Status().Lead
	return types.ID(lead)
}

// IsLeader determine whether the current node is leader
func (b *BinlogSyncer) IsLeader() bool {
	leaderId := b.Leader()
	if types.ID(b.config.RaftNodeConfig.Id) == leaderId {
		return true
	}
	return false
}

// StopSyncer implements that stop the specified syncer
func (b *BinlogSyncer) StopSyncer(syncerId uint32) {
	b.canals[syncerId].Close()
	b.syncerMeta[syncerId] = STOP
}

// StartSyncer implements that start the syncer
func (b *BinlogSyncer) StartSyncer(cfg *config.SyncerHandleConfig) error {
	if b.syncerMeta[cfg.ServerID] != STOP {
		return ErrStatusStop
	}

	b.updateSyncerConfig(cfg)

	b.syncerMeta[cfg.ServerID] = START
	if err := b.NewCanal(cfg.ServerID); err != nil {
		log.Log.Errorf("StartSyncer: newCanal error, err: %s", err.Error())
		return err
	}

	if err := b.PrepareCanal(cfg.ServerID); err != nil {
		log.Log.Errorf("StartSyncer: PrepareCanal error, err: %s", err.Error())
		return err
	}

	b.syncerMeta[cfg.ServerID] = RUNNING
	return nil
}

// UpdateSyncer implements that update the specified syncer and restart
func (b *BinlogSyncer) UpdateSyncer(cfg *config.SyncerHandleConfig) error {
	// 1. set status
	b.syncerMeta[cfg.ServerID] = UPDATE

	// 2. stop specified syncer
	b.StopSyncer(cfg.ServerID)

	// 3. update config
	b.updateSyncerConfig(cfg)

	// 4. start new syncer
	err := b.StartSyncer(cfg)

	if err != nil {
		log.Log.Errorf("UpdateSyncer: startSyncer error : err %s", err.Error())
		return err
	}
	return nil
}

// GetSyncerStatus returns the all syncer configuration and status.
func (s *BinlogSyncer) GetSyncersStatus() interface{} {
	return s.syncerMeta
}

// updateSyncer update syncer config
func (s *BinlogSyncer) updateSyncerConfig(cfg *config.SyncerHandleConfig) {
	sc := &config.SyncerConfig{
		MysqlAddr:      cfg.MysqlAddr,
		MysqlUser:      cfg.MysqlUser,
		MysqlPassword:  cfg.MysqlPassword,
		MysqlCharset:   cfg.MysqlCharset,
		MysqlPosition:  cfg.MysqlPosition,
		ServerID:       cfg.ServerID,
		Flavor:         cfg.Flavor,
		DataDir:        cfg.DataDir,
		DumpExec:       cfg.DumpExec,
		SkipMasterData: cfg.SkipMasterData,
		Sources:        cfg.Sources,
		Rules:          cfg.Rules,
		SkipNoPkTable:  cfg.SkipNoPkTable,
	}
	s.config.SyncerConfigs[cfg.ServerID] = sc
}

// PrepareCancel pre initialization canal.
func (s *Server) PrepareCanal(syncerId uint32) error {
	var db string
	dbs := map[string]struct{}{}
	tables := make([]string, 0, len(s.rules))
	for _, rule := range s.rules {
		db = rule.Schema
		dbs[rule.Schema] = struct{}{}
		tables = append(tables, rule.Table)
	}

	if len(db) == 1 {
		// one db, we can shrink using table
		s.canals[syncerId].AddDumpTables(db, tables...)
	} else {
		// many dbs, can only assign databases to dumo
		keys := make([]string, 0, len(dbs))
		for key := range dbs {
			keys = append(keys, key)
		}
		s.canals[syncerId].AddDumpDatabases(keys...)
	}

	s.canals[syncerId].SetEventHandler(&eventHandler{s: s})

	m := s.syncerMeta
	m[s.config.SyncerConfigs[syncerId].ServerID] = PREPARE

	return nil
}

// NewCancel creates a canal ready to start.
func (s *Server) NewCanal(syncerId uint32) error {
	syncerConfig := s.config.SyncerConfigs[syncerId]

	if syncerConfig == nil {
		return ErrRuleNotExist
	}

	cfg := canal.NewDefaultConfig()
	cfg.Addr = syncerConfig.MysqlAddr
	cfg.User = syncerConfig.MysqlUser
	cfg.Password = syncerConfig.MysqlPassword
	cfg.Charset = syncerConfig.MysqlCharset
	cfg.Flavor = syncerConfig.Flavor

	cfg.ServerID = syncerConfig.ServerID
	cfg.Dump.ExecutionPath = syncerConfig.DumpExec
	cfg.Dump.DiscardErr = false
	cfg.Dump.SkipMasterData = syncerConfig.SkipMasterData

	for _, s := range s.config.SyncerConfigs[syncerId].Sources {
		for _, t := range s.Tables {
			cfg.IncludeTableRegex = append(cfg.IncludeTableRegex, s.Schema+"\\."+t)
		}
	}

	var err error
	s.canals[syncerId], err = canal.NewCanal(cfg)
	return errors.Trace(err)
}

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
	datas := make([]*syncer.RowData, 0, len(rows))

	for _, values := range rows {
		pd, err := s.makeProcessorData(rule, values)
		if err != nil {
			s.cancel()
			return errors.Errorf("assembly data error, err:%s", err)
		}

		data := &syncer.RowData{
			Action: action,
			Schema: rule.Schema,
			Table:  rule.Table,
			Data:   pd,
		}

		datas = append(datas, data)
	}

	fmt.Printf("binlog : %v", datas)
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

// syncLoop main method
func (s *Server) syncLoop() {
	defer s.wg.Done()

	lastSavedTime := time.Now()

	var pos mysql.Position

	for {
		needSavePos := false
		select {
		case v := <-s.syncCh:
			switch v := v.(type) {
			case posSaver:
				now := time.Now()
				if v.force || now.Sub(lastSavedTime) > 3*time.Second {
					lastSavedTime = now
					needSavePos = true
					pos = v.pos
				}
			}
		case <-s.ctx.Done():
			return
		}
		if needSavePos {
			if err := s.master.Save(pos); err != nil {
				log.Log.Errorf("save syncer position %s err %v, close sync", pos, err)
				s.cancel()
				return
			}
		}
	}
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
				log.Log.Warnf("invalid binlog enum index %d, for enum %v", eNum, col.EnumValues)
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
