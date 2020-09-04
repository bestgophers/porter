package main

import (
	"flag"
	"github.com/coreos/etcd/raft/raftpb"
	"porter/storage"
	"porter/syncer"
	"strings"
)

func main() {
	cluster := flag.String("cluster", "http://127.0.0.1:9021", "comma separated cluster peers")
	id := flag.Int("id", 1, "node ID")
	kvport := flag.Int("port", 9121, "key-value server port")
	join := flag.Bool("join", false, "join an existing cluster")
	flag.Parse()

	proposeC := make(chan string)
	defer close(proposeC)
	confChangeC := make(chan raftpb.ConfChange)
	defer close(confChangeC)

	var kvs *storage.KvStore
	getSnapshot := func() ([]byte, error) {
		return kvs.GetSnapshot()
	}
	commitC, errorC, snapshotters := NewRaftNodeTest(*id, strings.Split(*cluster, ","), *join, getSnapshot, proposeC, confChangeC)

	kvs = storage.NewKVStore(<-snapshotters, proposeC, commitC, errorC)
	ServeHttpKVAPI(kvs, *kvport, confChangeC, errorC)
	syncer.Test()

}
