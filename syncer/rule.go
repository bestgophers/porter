package syncer

import (
	"github.com/siddontang/go-mysql/schema"
	"strings"
)

type Rule struct {
	Schema string   `json:"schema"`
	Table  string   `json:"table"`
	Index  string   `json:"index"`
	Type   string   `json:"type"`
	Parent string   `json:"parent"`
	ID     []string `json:"id"`

	FieldMapping map[string]string `json:"field"`

	// Mysql table information
	TableInfo *schema.Table

	// only Mysql fields in filter will be synced, default sync all fields
	Filter []string `json:"filter"`

	Pipeline string `json:"pipeline"`
}

// newDefaultRule construct a default rule without filter
func NewDefaultRule(schema string, table string) *Rule {
	return newRule(schema, table, make(map[string]string))
}

// neRule  construct rule
func newRule(schema, table string, filter map[string]string) *Rule {
	return &Rule{
		Schema:       schema,
		Table:        table,
		Index:        strings.ToLower(table),
		Type:         strings.ToLower(table),
		FieldMapping: filter,
	}
}

// prepare pre initializer some field
func (r *Rule) Prepare() error {
	if r.FieldMapping == nil {
		r.FieldMapping = make(map[string]string)
	}
	if len(r.Index) == 0 {
		r.Index = r.Table
	}
	if len(r.Type) == 0 {
		r.Type = r.Index
	}
	return nil
}

// checkFilter checkers whether the field needs to filter.
func (r *Rule) CheckFilter(filed string) bool {
	if r.Filter == nil {
		return true
	}
	for _, f := range r.Filter {
		if f == filed {
			return true
		}
	}
	return false
}
