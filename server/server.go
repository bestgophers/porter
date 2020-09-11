package server

import (
	"context"
	"fmt"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/pingcap/errors"
	"github.com/siddontang/go-mysql/canal"
	"porter/api"
	"porter/config"
	"porter/log"
	pr "porter/raft"
	"porter/storage"
	"porter/syncer"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	DialTimeout = 5 * time.Second
)

// ErrRuleNotExist is the error if rule is not defined.
var ErrRuleNotExist = errors.New("rule is not exist")

type Server struct {
	syncerMeta map[uint32]SyncerType
	config     *config.PorterConfig
	adminSvr   *api.AdminServer
	canals     map[uint32]*canal.Canal
	rules      map[string]*syncer.Rule
	ctx        context.Context
	cancel     context.CancelFunc

	wg sync.WaitGroup

	master *masterInfo

	syncCh chan interface{}
}

// NewServer creates the Server form config
func NewServer(config *config.PorterConfig) (*Server, error) {
	s := new(Server)

	s.syncerMeta = make(map[uint32]SyncerType)
	s.config = config
	s.canals = make(map[uint32]*canal.Canal)
	s.rules = make(map[string]*syncer.Rule)
	s.syncCh = make(chan interface{}, 4096)
	s.ctx, s.cancel = context.WithCancel(context.Background())

	var err error

	//if err = s.startRaftNode(); err != nil {
	//	return nil, errors.Trace(err)
	//}
	go func() {
		s.startRaftNode()
	}()

	if err = s.startAdminServer(config.AdminURLs); err != nil {
		return nil, errors.Trace(err)
	}

	if s.master, err = loadMasterInfo(config.LogDir); err != nil {
		return nil, errors.Trace(err)
	}

	//if err = s.NewCanal(config.ServerID); err != nil {
	//	return nil, errors.Trace(err)
	//}
	//
	//if err = s.prepareRule(config.ServerID); err != nil {
	//	return nil, errors.Trace(err)
	//}
	//
	//if err = s.PrepareCanal(config.ServerID); err != nil {
	//	return nil, errors.Trace(err)
	//}
	//
	//if err = s.canals[config.ServerID].CheckBinlogRowImage("FULL"); err != nil {
	//	return nil, errors.Trace(err)
	//}
	return s, nil
}

func (s *Server) startRaftNode() error {
	proposeC := make(chan string)
	defer close(proposeC)
	confChangeC := make(chan raftpb.ConfChange)
	defer close(confChangeC)

	var kvs *storage.KvStore
	getSnapshot := func() ([]byte, error) {
		return kvs.GetSnapshot()
	}

	commitC, errorC, snapshotters := pr.NewRaftNode(&s.config.RaftNodeConfig, getSnapshot, proposeC, confChangeC)

	kvs = storage.NewKVStore(<-snapshotters, proposeC, commitC, errorC)
	pr.ServeHttpKVAPI(kvs, s.config.RaftNodeConfig.Port, confChangeC, errorC)

	return nil
}

func (s *Server) startAdminServer(url string) error {
	s.adminSvr = api.NewAdminServer(url, NewBinlogSyncer(s))
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
	tableInfo, err := s.canals[s.config.ServerID].GetTable(schema, table)
	if err != nil {
		return errors.Trace(err)
	}
	rule.TableInfo = tableInfo

	return nil
}

func (se *Server) parseSource() (map[string][]string, error) {
	syncer := se.config.SyncerConfigs[se.config.ServerID]
	wildTables := make(map[string][]string, len(syncer.Sources))

	// first, check source
	for _, s := range syncer.Sources {
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

				res, err := se.canals[syncer.ServerID].Execute(sql)
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

func (s *Server) prepareRule(syncerId uint32) error {
	syn := s.config.SyncerConfigs[syncerId]
	wildtables, err := s.parseSource()
	if err != nil {
		return errors.Trace(err)
	}

	if syn.Rules != nil {
		for _, rule := range syn.Rules {
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
		if rule.TableInfo, err = s.canals[syncerId].GetTable(rule.Schema, rule.Table); err != nil {
			return errors.Trace(err)
		}

		if len(rule.TableInfo.PKColumns) == 0 {
			if !syn.SkipNoPkTable {
				return errors.Errorf("%s.%s must have a PK for a column", rule.Schema, rule.Table)
			}

			log.Log.Errorf("ignored table without a primary key : %s\n", rule.TableInfo.Name)
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

const STATUS = "StateLeader"

// Run syncs the data from mysql and process.
func (s *Server) Run() error {
	go s.adminSvr.Run()
	//nodeConfig := s.config.RaftNodeConfig
	//node := nodeConfig.Node
	//status := node.Status()
	//s2 := s.config.RaftNodeConfig.Node.Status().RaftState.String()
	//
	//fmt.Printf("%v", status)
	//if s2 != STATUS {
	//	return nil
	//}
	//
	//s.wg.Add(1)
	//
	//go s.syncLoop()
	//
	//for _, sc := range s.config.SyncerConfigs {
	//	position := s.master.Position()
	//	if err := s.canals[sc.ServerID].RunFrom(position); err != nil {
	//		log.Log.Errorf("start canal err %v", err)
	//		return errors.Trace(err)
	//	}
	//}
	return nil
}

// Ctx returns the internal context for outside use.
func (s *Server) Ctx() context.Context {
	return s.ctx
}

func (s *Server) Close() {
	log.Log.Infof("closing porter server")
	s.cancel()
	for _, canal := range s.canals {
		canal.Close()
	}
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
