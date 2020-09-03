package storage

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"github.com/coreos/etcd/snap"
	"log"
	"sync"
)

// A key-value store backed by raft
type KvStore struct {
	ProposeC    chan<- string // channel for proposing updates
	Mu          sync.RWMutex
	KvStore     map[string]string // current committed key-value pairs
	Snapshotter *snap.Snapshotter
}

type Kv struct {
	Key string
	Val string
}

func NewKVStore(snapshotter *snap.Snapshotter, proposeC chan<- string, commitC <-chan *string, errorC <-chan error) *KvStore {
	store := &KvStore{
		ProposeC:    proposeC,
		KvStore:     make(map[string]string),
		Snapshotter: snapshotter,
	}
	// replay log into key-value map
	store.readCommits(commitC, errorC)

	// read commits from raft into kvstore map util err
	go store.readCommits(commitC, errorC)
	return store
}

func (k *KvStore) Lookup(key string) (string, bool) {
	k.Mu.Lock()
	defer k.Mu.Unlock()
	v, ok := k.KvStore[key]
	return v, ok
}

func (k *KvStore) Propose(key, v string) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(Kv{Key: key, Val: v}); err != nil {
		log.Fatal(err)
	}
	k.ProposeC <- buf.String()
}

func (k *KvStore) readCommits(commitC <-chan *string, errorC <-chan error) {
	for data := range commitC {
		if data == nil {
			// done replaying log; new data incoming
			// or signaled to load snapshot
			snapshot, err := k.Snapshotter.Load()
			if err == snap.ErrNoSnapshot {
				return
			}
			if err != nil {
				log.Panic(err)
			}
			log.Printf("loading snapshot at term %d and index %d", snapshot.Metadata.Term, snapshot.Metadata.Index)
			if err := k.recoverFromSnapshot(snapshot.Data); err != nil {
				log.Panic(err)
			}
			continue
		}
		var dataKv Kv
		dec := gob.NewDecoder(bytes.NewBufferString(*data))
		if err := dec.Decode(&dataKv); err != nil {
			log.Fatalf("raft: could not decode message (%v)", err)
		}
		k.Mu.Lock()
		k.KvStore[dataKv.Key] = dataKv.Val
		k.Mu.Unlock()
	}
	if err, ok := <-errorC; ok {
		log.Fatal(err)
	}
}

func (k *KvStore) GetSnapshot() ([]byte, error) {
	k.Mu.RLock()
	defer k.Mu.RUnlock()
	return json.Marshal(k.KvStore)
}

func (k *KvStore) recoverFromSnapshot(snapshot []byte) error {
	var store map[string]string
	if err := json.Unmarshal(snapshot, &store); err != nil {
		return err
	}

	k.Mu.Lock()
	defer k.Mu.Unlock()
	k.KvStore = store
	return nil
}
