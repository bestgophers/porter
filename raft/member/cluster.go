package member

import (
	"github.com/coreos/etcd/pkg/types"
	"github.com/pingcap/goleveldb/leveldb/storage"
	"sync"
)

type RaftCluster struct {
	Id    types.ID        `json:"id"`
	Store storage.Storage `json:"-"`

	sync.Mutex `json:"-"`
	MemberMap  map[types.ID]*Member `json:"member_map"`
	RemovedMap map[types.ID]bool    `json:"removed_map"`
}
