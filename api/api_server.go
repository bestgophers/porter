package api

import "github.com/coreos/etcd/pkg/types"

type Server interface {
	Leader() types.ID

	IsLeader() bool

	StopSyncer(args interface{})

	StartSyncer(args interface{}) error

	UpdateSyncer(args interface{}) error

	GetSyncersStatus() interface{}
}
