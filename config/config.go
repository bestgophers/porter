package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/rafthttp"
	"github.com/coreos/etcd/snap"
	"github.com/coreos/etcd/wal"
	"github.com/juju/errors"
	"io/ioutil"
	"porter/syncer"
)

type PorterConfig struct {
	RaftNodeConfig
	SyncerConfigs map[uint32]*SyncerConfig

	ServerID    uint32 `toml:"server_id"`
	AdminURLs   string `toml:"admin_url"`
	MetricsAddr string `toml:"stat_addr"`
	LogDir      string `toml:"log_dir"`
	LogLevel    string `toml:"log_level""`
}

type SourceConfig struct {
	Schema string   `toml:"schema"`
	Tables []string `toml:"tables"`
}
type SyncerConfig struct {
	MysqlAddr     string `toml:"mysql_addr"`
	MysqlUser     string `toml:"mysql_user"`
	MysqlPassword string `toml:"mysql_pass"`
	MysqlCharset  string `toml:"mysql_charset"`
	MysqlPosition int    `toml:mysql_position`

	ServerID uint32 `toml:"server_id"`
	Flavor   string `toml:"flavor"`
	DataDir  string `toml:"data_dir"`

	DumpExec       string `toml:"mysqldump""`
	SkipMasterData bool   `toml:"skip_master_data"`

	Sources       []SourceConfig `toml:"source"`
	Rules         []*syncer.Rule `toml:"rule"`
	SkipNoPkTable bool           `toml:"skip_no_pk_table"`
}

// A key-value stream backed by raft
type RaftNodeConfig struct {
	ProposeC    <-chan string            // proposed message (k,v)
	ConfChangeC <-chan raftpb.ConfChange // proposed cluster config changes
	CommitC     chan<- *string           // entries committed to log (k,v)
	ErrorC      chan<- error             // errors from raft session

	Id      int      `toml:"raft_id"`       // cluster ID for raft session
	Peers   []string `toml:"raft_cluster"`  // raft peer URLs
	Join    bool     `toml:"raft_join"`     // node is joining an existing cluster
	Waldir  string   `toml:"raft_wal_dir"`  // path to WAL directory
	Snapdir string   `toml:"raft_snap_dir"` // path to snapshot directory
	Port    int      `toml:"raft_port"`

	GetSnapshot func() ([]byte, error)
	LastIndex   uint64 // index of log at start

	ConfState     raftpb.ConfState
	SnapshotIndex uint64
	AppliedIndex  uint64

	// raft backing for the commit/error channel
	Node        raft.Node
	RaftStorage *raft.MemoryStorage
	Wal         *wal.WAL

	Snapshotter      *snap.Snapshotter
	SnapshotterReady chan *snap.Snapshotter // signals when snapshotter is ready

	SnapCount uint64
	Transport *rafthttp.Transport
	Stopc     chan struct{} // signals proposal channel closed
	Httpstopc chan struct{} // signals http server to shutdown
	Httpdonec chan struct{} // signals http server shutdown complete
}

// NewPorterConfig implements create a config of porter server
func NewPorterConfig(configPath string) (*PorterConfig, error) {

	if configPath == "" || len(configPath) == 0 {
		return nil, errors.New("config.NewPorterConfig error, err: configpath is nil")
	}

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewConfig(string(data))
}

// NewConfig creates a Config from data
func NewConfig(data string) (*PorterConfig, error) {
	var cfg PorterConfig

	_, err := toml.Decode(data, &cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg.buildRafeNodeConfig()
	syncerConfig, err := getSyncerConfig(data)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg.SyncerConfigs[cfg.ServerID] = syncerConfig
	return &cfg, nil
}

func getSyncerConfig(data string) (*SyncerConfig, error) {
	var cfg SyncerConfig

	_, err := toml.Decode(data, &cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &cfg, nil
}

func (cfg *PorterConfig) buildRafeNodeConfig() {
	cfg.SyncerConfigs = make(map[uint32]*SyncerConfig, 12)

	cfg.LogDir += fmt.Sprintf("/porter-%d", cfg.RaftNodeConfig.Id)

	cfg.RaftNodeConfig.Waldir = cfg.LogDir + fmt.Sprintf(cfg.RaftNodeConfig.Waldir+"-%d", cfg.RaftNodeConfig.Id)
	cfg.RaftNodeConfig.Snapdir = cfg.LogDir + fmt.Sprintf(cfg.RaftNodeConfig.Snapdir+"-%d", cfg.RaftNodeConfig.Id)

	cfg.LogDir += "/logs"

	cfg.RaftNodeConfig.SnapCount = 10000
	cfg.RaftNodeConfig.Stopc = make(chan struct{})
	cfg.RaftNodeConfig.Httpstopc = make(chan struct{})
	cfg.RaftNodeConfig.Httpdonec = make(chan struct{})
	cfg.RaftNodeConfig.SnapshotterReady = make(chan *snap.Snapshotter, 1)
}
