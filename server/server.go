package server

import (
	"context"
	"fmt"
	"github.com/pingcap/errors"
	"github.com/prometheus/common/log"
	"github.com/siddontang/go-mysql/canal"
	"porter/config"
	"porter/syncer"
	"regexp"
	"strings"
	"sync"
)

// ErrRuleNotExist is the error if rule is not defined.
var ErrRuleNotExist = errors.New("rule is not exist")

type Server struct {
	config *config.PorterConfig
	canal  *canal.Canal
	rules  map[string]*syncer.Rule
	ctx    context.Context
	cancel context.CancelFunc

	wg sync.WaitGroup

	master *masterInfo

	syncCh chan interface{}
}

// NewServer creates the Server form config
func NewServer(config *config.PorterConfig) (*Server, error) {
	s := new(Server)

	s.config = config
	s.rules = make(map[string]*syncer.Rule)
	s.syncCh = make(chan interface{}, 4096)
	s.ctx, s.cancel = context.WithCancel(context.Background())

	var err error
	if s.master, err = loadMasterInfo(config.LogDir); err != nil {
		return nil, errors.Trace(err)
	}

	if err = s.newCanal(); err != nil {
		return nil, errors.Trace(err)
	}

	if err = s.prepareRule(); err != nil {
		return nil, errors.Trace(err)
	}

	if err = s.prepareCanal(); err != nil {
		return nil, errors.Trace(err)
	}

	if err = s.canal.CheckBinlogRowImage("FULL"); err != nil {
		return nil, errors.Trace(err)
	}

	return s, nil
}

func (s *Server) newCanal() error {
	cfg := canal.NewDefaultConfig()
	cfg.Addr = s.config.SyncerConfig.MysqlAddr
	cfg.User = s.config.SyncerConfig.MysqlUser
	cfg.Password = s.config.SyncerConfig.MysqlPassword
	cfg.Charset = s.config.SyncerConfig.MysqlCharset
	cfg.Flavor = s.config.SyncerConfig.Flavor

	cfg.ServerID = s.config.SyncerConfig.ServerID
	cfg.Dump.ExecutionPath = s.config.SyncerConfig.DumpExec
	cfg.Dump.DiscardErr = false
	cfg.Dump.SkipMasterData = s.config.SyncerConfig.SkipMasterData

	for _, s := range s.config.Sources {
		for _, t := range s.Tables {
			cfg.IncludeTableRegex = append(cfg.IncludeTableRegex, s.Schema+"\\."+t)
		}
	}

	var err error
	s.canal, err = canal.NewCanal(cfg)
	return errors.Trace(err)
}

func (s *Server) prepareCanal() error {
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
		s.canal.AddDumpTables(db, tables...)
	} else {
		// many dbs, can only assign databases to dumo
		keys := make([]string, 0, len(dbs))
		for key := range dbs {
			keys = append(keys, key)
		}
		s.canal.AddDumpDatabases(keys...)
	}
	s.canal.SetEventHandler(&eventHandler{s: s})
	return nil
}

func (s *Server) newRule(schema, table string) error {
	key := ruleKey(schema, table)

	if _, ok := s.rules[key]; ok {
		return errors.Errorf("duplicate source %s, %s defined in config", schema, table)
	}
	s.rules[key] = syncer.NewDefaultRule(schema, table)
	return nil
}

func (s *Server) updateRule(schema, table string) error {
	rule, ok := s.rules[ruleKey(schema, table)]
	if !ok {
		return ErrRuleNotExist
	}
	tableInfo, err := s.canal.GetTable(schema, table)
	if err != nil {
		return errors.Trace(err)
	}
	rule.TableInfo = tableInfo

	return nil
}

func (se *Server) parseSource() (map[string][]string, error) {
	wildTables := make(map[string][]string, len(se.config.Sources))

	// first, check source
	for _, s := range se.config.Sources {
		if !isValidTables(s.Tables) {
			return nil, errors.Errorf("wildcard * is not allowed for multiple tables")
		}

		for _, table := range s.Tables {
			if len(s.Schema) == 0 {
				return nil, errors.Errorf("empty schema not allowed for source")
			}
			if regexp.QuoteMeta(table) != table {
				if _, ok := wildTables[ruleKey(s.Schema, table)]; ok {
					return nil, errors.Errorf("duplicate wildcard table defined for %s.%s", s.Schema, table)
				}

				tables := []string{}

				sql := fmt.Sprintf(`SELECT table_name FROM information_schema.tables WHERE
					table_name RLIKE "%s" AND table_schema = "%s";`, buildTable(table), s.Schema)

				res, err := se.canal.Execute(sql)
				if err != nil {
					return nil, errors.Trace(err)
				}

				for i := 0; i < res.Resultset.RowNumber(); i++ {
					f, _ := res.GetString(i, 0)
					err := se.newRule(s.Schema, f)
					if err != nil {
						return nil, errors.Trace(err)
					}

					tables = append(tables, f)
				}
				wildTables[ruleKey(s.Schema, table)] = tables
			} else {
				err := se.newRule(s.Schema, table)
				if err != nil {
					return nil, errors.Trace(err)
				}
			}
		}
	}
	if len(se.rules) == 0 {
		return nil, errors.Errorf("no source data defined")
	}
	return wildTables, nil
}

func (s *Server) prepareRule() error {
	wildtables, err := s.parseSource()
	if err != nil {
		return errors.Trace(err)
	}

	if s.config.Rules != nil {
		for _, rule := range s.config.Rules {
			if len(rule.Schema) == 0 {
				return errors.Errorf("empty schema not allowed for rule")
			}
			if regexp.QuoteMeta(rule.Table) != rule.Table {
				// wildcard table
				tables, ok := wildtables[ruleKey(rule.Schema, rule.Table)]
				if !ok {
					return errors.Errorf("wildcard table for %s.%s is not defined in source", rule.Schema, rule.Table)
				}

				if len(rule.Index) == 0 {
					return errors.Errorf("wildcard table rule %s.%s must have index, can not empty", rule.Schema, rule.Table)
				}

				rule.Prepare()

				for _, table := range tables {
					rr := s.rules[ruleKey(rule.Schema, table)]
					rr.Index = rule.Index
					rr.Type = rule.Index
					rr.Parent = rule.Parent
					rr.ID = rule.ID
					rr.FieldMapping = rule.FieldMapping
				}
			} else {
				key := ruleKey(rule.Schema, rule.Table)
				if _, ok := s.rules[key]; !ok {
					return errors.Errorf("rule %s, %s not defined in source", rule.Schema, rule.Table)
				}
				rule.Prepare()
				s.rules[key] = rule
			}
		}
	}

	rules := make(map[string]*syncer.Rule)
	for key, rule := range s.rules {
		if rule.TableInfo, err = s.canal.GetTable(rule.Schema, rule.Table); err != nil {
			return errors.Trace(err)
		}

		if len(rule.TableInfo.PKColumns) == 0 {
			if !s.config.SkipNoPkTable {
				return errors.Errorf("%s.%s must have a PK for a column", rule.Schema, rule.Table)
			}

			log.Errorf("ignored table without a primary key : %s\n", rule.TableInfo.Name)
		} else {
			rules[key] = rule
		}
	}
	s.rules = rules
	return nil
}

func ruleKey(schema, table string) string {
	return strings.ToLower(fmt.Sprintf("%s:%s", schema, table))
}

// Run syncs the data from mysql and process.
func (s *Server) Run() error {
	s.wg.Add(1)

	go s.syncLoop()

	position := s.master.Position()
	if err := s.canal.RunFrom(position); err != nil {
		log.Errorf("start canal err %v", err)
		return errors.Trace(err)
	}
	return nil
}

// Ctx returns the internal context for outside use.
func (s *Server) Ctx() context.Context {
	return s.ctx
}

func (s *Server) Close() {
	log.Infof("closing porter server")
	s.cancel()
	s.canal.Close()
	s.master.Close()
	s.wg.Wait()
}

// isValidTables checkers whether the table valid, currently don't support * and only specified table
func isValidTables(tables []string) bool {
	if len(tables) > 1 {
		for _, table := range tables {
			if table == "*" {
				return false
			}
		}
	}
	return true
}

func buildTable(table string) string {
	if table == "*" {
		return "." + table
	}
	return table
}
