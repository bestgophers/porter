package api

import "github.com/coreos/etcd/pkg/types"

type Server interface {
	Leader() types.ID
}
