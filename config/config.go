package config

import (
	"github.com/BurntSushi/toml"
	"github.com/coreos/etcd/pkg/types"
	"github.com/juju/errors"
	"io/ioutil"
	"porter/syncer"
)

type PorterConfig struct {
	RaftNodeConfig
	SyncerConfig

	AdminURLs   types.URLs
	MetricsAddr string
	LogDir      string
	LogLevel    string
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

	ServerID uint32 `toml:"server_id"`
	Flavor   string `toml:"flavor"`
	DataDir  string `toml:"data_dir"`

	DumpExec       string `toml:"mysqldump""`
	SkipMasterData bool   `toml:"skip_master_data"`

	Sources       []SourceConfig `toml:"source"`
	Rules         []*syncer.Rule `toml:"rule"`
	SkipNoPkTable bool           `toml:"skip_no_pk_table"`
}

type RaftNodeConfig struct {
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
	//viper.SetConfigFile(configPath)
	//if err := viper.ReadInConfig(); err != nil {
	//	return nil, err
	//}
	//
	//// get raft node config
	//raftNodeCfg, err := getRaftNodeConfig()
	//if err != nil {
	//	return nil, err
	//}
	//
	//// get Syncer config
	//syncerCfg, err := getSyncerConfig()
	//if err != nil {
	//	return nil, err
	//}
	//
	//adminAddr, err := types.NewURLs([]string{viper.GetString("admin-url")})
	//if err != nil {
	//	return nil, err
	//}
	//if len(adminAddr) != 1 {
	//	return nil, fmt.Errorf("adminURLs must be 1")
	//}
	//
	//config := &PorterConfig{
	//	RaftNodeConfig: *raftNodeCfg,
	//	SyncerConfig:   *syncerCfg,
	//	AdminURLs:      adminAddr,
	//	MetricsAddr:    viper.GetString("metrics-addr"),
	//	LogDir:         viper.GetString("log-dir"),
	//	LogLevel:       viper.GetString("log-level"),
	//}
	//return config, nil
}

// NewConfig creates a Config from data
func NewConfig(data string) (*PorterConfig, error) {
	var cfg PorterConfig

	_, err := toml.Decode(data, &cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &cfg, nil
}

func getRaftNodeConfig() (*RaftNodeConfig, error) {
	raftNodeCfg := &RaftNodeConfig{}
	return raftNodeCfg, nil
}

func getSyncerConfig() (*SyncerConfig, error) {
	syncerCfg := &SyncerConfig{}
	return syncerCfg, nil
}
