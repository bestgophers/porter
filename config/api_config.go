package config

import "porter/syncer"

type SyncerHandleConfig struct {
	MysqlAddr     string `json:"mysql_addr"`
	MysqlUser     string `json:"mysql_user"`
	MysqlPassword string `json:"mysql_password"`
	MysqlCharset  string `json:"mysql_charset"`
	MysqlPosition int    `json:mysql_position`

	ServerID uint32 `json:"server_id"`
	Flavor   string `json:"flavor"`
	DataDir  string `json:"data_dir"`

	DumpExec       string `json:"mysqldump""`
	SkipMasterData bool   `json:"skip_master_data"`

	Sources       []SourceConfig `json:"sources"`
	Rules         []*syncer.Rule `json:"rules"`
	SkipNoPkTable bool           `json:"skip_no_pk_table"`
}
