package raft

import (
	"context"
	"github.com/coreos/etcd/etcdserver/stats"
	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/coreos/etcd/pkg/types"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/rafthttp"
	"github.com/coreos/etcd/snap"
	"github.com/coreos/etcd/wal"
	"github.com/coreos/etcd/wal/walpb"
	"log"
	"net/http"
	"net/url"
	"os"
	"porter/config"
	"strconv"
	"time"
)

type RaftNode struct {
	config.RaftNodeConfig
}

func NewRaftNode(rnc *config.RaftNodeConfig, getSnapshot func() ([]byte, error), proposeC <-chan string,
	confChangeC <-chan raftpb.ConfChange) (<-chan *string, <-chan error, <-chan *snap.Snapshotter) {

	commitC := make(chan *string)
	errorC := make(chan error)

	rnc.ProposeC = proposeC
	rnc.ConfChangeC = confChangeC
	rnc.CommitC = commitC
	rnc.ErrorC = errorC
	rnc.GetSnapshot = getSnapshot

	node := RaftNode{*rnc}

	go node.startRaft()
	return commitC, errorC, rnc.SnapshotterReady
}

const STATE_LEADER = "StateLeader"

func (r *RaftNode) IsLeader() bool {
	if r.Node.Status().RaftState.String() != STATE_LEADER {
		return false
	}
	return true
}

func (rc *RaftNode) saveSnap(snap raftpb.Snapshot) error {
	walSnap := walpb.Snapshot{
		Index: snap.Metadata.Index,
		Term:  snap.Metadata.Term,
	}

	if err := rc.Wal.SaveSnapshot(walSnap); err != nil {
		return err
	}

	if err := rc.Snapshotter.SaveSnap(snap); err != nil {
		return err
	}
	return rc.Wal.ReleaseLockTo(snap.Metadata.Index)
}

func (rc *RaftNode) entriesToApply(ents []raftpb.Entry) (nets []raftpb.Entry) {
	if len(ents) == 0 {
		return ents
	}
	firstIndex := ents[0].Index
	if firstIndex > rc.AppliedIndex+1 {
		log.Fatalf("first index of committed entry[%d] should <= progress.appliedIndex[%d]+1", firstIndex, rc.AppliedIndex)
	}
	if rc.AppliedIndex-firstIndex+1 < uint64(len(ents)) {
		nets = ents[rc.AppliedIndex-firstIndex+1:]
	}
	return nets
}

func (rc *RaftNode) publishEntries(ents []raftpb.Entry) bool {
	for i := range ents {
		switch ents[i].Type {
		case raftpb.EntryNormal:
			if len(ents[i].Data) == 0 {
				// ignore empty messages
				break
			}
			s := string(ents[i].Data)
			select {
			case rc.CommitC <- &s:
			case <-rc.Stopc:
				return false
			}
		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			cc.Unmarshal(ents[i].Data)
			rc.ConfState = *rc.Node.ApplyConfChange(cc)
			switch cc.Type {
			case raftpb.ConfChangeAddNode:
				if len(cc.Context) > 0 {
					rc.Transport.AddPeer(types.ID(cc.NodeID), []string{string(cc.Context)})
				}
			case raftpb.ConfChangeRemoveNode:
				if cc.NodeID == uint64(rc.Id) {
					log.Println("I've been removed from the cluster ! shutting down")
					return false
				}
				rc.Transport.RemovePeer(types.ID(cc.NodeID))
			}
		}

		// after commit ,update appliedIndex
		rc.AppliedIndex = ents[i].Index

		// special nil commit to signal replay has finished
		if ents[i].Index == rc.LastIndex {
			select {
			case rc.CommitC <- nil:
			case <-rc.Stopc:
				return false
			}
		}
	}
	return true
}

func (rc *RaftNode) loadSnapshot() *raftpb.Snapshot {
	load, err := rc.Snapshotter.Load()
	if err != nil && err != snap.ErrNoSnapshot {
		log.Fatalf("raft: error loading snapshot (%v)", err)
	}
	return load
}

// openWAL returns a WAL ready for reading
func (rc *RaftNode) openWAL(snapshot *raftpb.Snapshot) *wal.WAL {
	if !wal.Exist(rc.Waldir) {
		if err := os.Mkdir(rc.Waldir, 0750); err != nil {
			log.Fatalf("raft: cannot create dir from wal (%v)", err)
		}

		w, err := wal.Create(rc.Waldir, nil)
		if err != nil {
			log.Fatalf("raft: create wal err (%v)", err)
		}
		w.Close()
	}

	walsnap := walpb.Snapshot{}
	if snapshot != nil {
		walsnap.Index, walsnap.Term = snapshot.Metadata.Index, snapshot.Metadata.Term
	}
	log.Printf("loading WAL at term %d and index %d", walsnap.Term, walsnap.Index)
	w, err := wal.Open(rc.Waldir, walsnap)
	if err != nil {
		log.Fatalf("raft: error loading wal (%v)", err)
	}
	return w
}

// replayWAL replays WAL entries into the raft instance
func (rc *RaftNode) replayWAL() *wal.WAL {
	log.Printf("replaying WAL of member %d", rc.Id)
	snapshot := rc.loadSnapshot()
	w := rc.openWAL(snapshot)
	_, st, ents, err := w.ReadAll()
	if err != nil {
		log.Fatalf("raft: failed to read WAL (%v)", err)
	}

	rc.RaftStorage = raft.NewMemoryStorage()
	if snapshot != nil {
		rc.RaftStorage.ApplySnapshot(*snapshot)
	}
	rc.RaftStorage.SetHardState(st)

	// append to storage so raft starts at the right place in log
	rc.RaftStorage.Append(ents)
	// send nil once lastIndex is published so client knows commit channel is current
	if len(ents) > 0 {
		rc.LastIndex = ents[len(ents)-1].Index
	} else {
		rc.CommitC <- nil
	}
	return w
}

func (rc *RaftNode) writeError(err error) {
	rc.stopHttp()
	close(rc.CommitC)
	rc.ErrorC <- err
	close(rc.ErrorC)
	rc.Node.Stop()
}

func (rc *RaftNode) startRaft() {
	if !fileutil.Exist(rc.Snapdir) {
		if err := os.Mkdir(rc.Snapdir, 0750); err != nil {
			log.Fatalf("raft: cannt crete dir for snapshot (%v)", err)
		}
	}
	rc.Snapshotter = snap.New(rc.Snapdir)
	rc.SnapshotterReady <- rc.Snapshotter

	oldWal := wal.Exist(rc.Waldir)
	rc.Wal = rc.replayWAL()

	rpeers := make([]raft.Peer, len(rc.Peers))
	for i := range rpeers {
		rpeers[i] = raft.Peer{ID: uint64(i + 1)}
	}
	c := &raft.Config{
		ID:              uint64(rc.Id),
		ElectionTick:    10,
		HeartbeatTick:   1,
		Storage:         rc.RaftStorage,
		MaxSizePerMsg:   1024 * 1024,
		MaxInflightMsgs: 256,
	}

	if oldWal {
		rc.Node = raft.RestartNode(c)
	} else {
		startPeers := rpeers
		if rc.Join {
			startPeers = nil
		}
		rc.Node = raft.StartNode(c, startPeers)
	}

	rc.Transport = &rafthttp.Transport{
		ID:          types.ID(rc.Id),
		ClusterID:   0x1000,
		Raft:        rc,
		ServerStats: stats.NewServerStats("", ""),
		LeaderStats: stats.NewLeaderStats(strconv.Itoa(rc.Id)),
		ErrorC:      make(chan error),
	}

	rc.Transport.Start()
	for i := range rc.Peers {
		if i+1 != rc.Id {
			rc.Transport.AddPeer(types.ID(i+1), []string{rc.Peers[i]})
		}
	}
	go rc.serveRaft()
	go rc.serveChannels()
}

// stop closed http, closes all channels, and stops raft.
func (rc *RaftNode) stop() {
	rc.stopHttp()
	close(rc.CommitC)
	close(rc.ErrorC)
	rc.Node.Stop()
}

func (rc *RaftNode) stopHttp() {
	rc.Transport.Stop()
	close(rc.Httpstopc)
	<-rc.Httpdonec
}

func (rc *RaftNode) publishSnapshot(snapshotToSave raftpb.Snapshot) {
	if raft.IsEmptySnap(snapshotToSave) {
		return
	}
	log.Printf("publishing snapshot at index %d", rc.SnapshotIndex)
	defer log.Printf("finished publishing snapshot at index %d", rc.SnapshotIndex)

	if snapshotToSave.Metadata.Index <= rc.AppliedIndex {
		log.Fatalf("snapshot index [%d] should > progress.appliedIndex [%d]", snapshotToSave.Metadata.Index, rc.AppliedIndex)
	}
	rc.CommitC <- nil // trigger kvstore to load snapshot

	rc.ConfState = snapshotToSave.Metadata.ConfState
	rc.SnapshotIndex = snapshotToSave.Metadata.Index
	rc.AppliedIndex = snapshotToSave.Metadata.Index
}

var snapshotCatchUpEntriesN uint64 = 10000

func (rc *RaftNode) maybeTriggerSnapshot() {
	if rc.AppliedIndex-rc.SnapshotIndex <= rc.SnapCount {
		return
	}
	log.Printf("start snapshot [applied index: %d | last snapshot index: %d]", rc.AppliedIndex, rc.SnapshotIndex)
	data, err := rc.GetSnapshot()
	if err != nil {
		log.Panic(err)
	}

	snapshot, err := rc.RaftStorage.CreateSnapshot(rc.AppliedIndex, &rc.ConfState, data)
	if err != nil {
		panic(err)
	}
	if err := rc.saveSnap(snapshot); err != nil {
		panic(err)
	}

	compactIndex := uint64(1)
	if rc.AppliedIndex > snapshotCatchUpEntriesN {
		compactIndex = rc.AppliedIndex - snapshotCatchUpEntriesN
	}
	if err := rc.RaftStorage.Compact(compactIndex); err != nil {
		panic(err)
	}
	log.Printf("compacted log at index %d", compactIndex)
	rc.SnapshotIndex = rc.AppliedIndex
}

func (rc *RaftNode) serveChannels() {
	snapshot, err := rc.RaftStorage.Snapshot()
	if err != nil {
		panic(err)
	}
	rc.ConfState = snapshot.Metadata.ConfState
	rc.SnapshotIndex = snapshot.Metadata.Index
	rc.AppliedIndex = snapshot.Metadata.Index

	defer rc.Wal.Close()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// send proposals over raft
	go func() {
		confChangeCount := uint64(0)

		for rc.ProposeC != nil && rc.ConfChangeC != nil {
			select {
			case prop, ok := <-rc.ProposeC:
				if !ok {
					rc.ProposeC = nil
				} else {
					// blocks utils accepted by raft state machine
					rc.Node.Propose(context.TODO(), []byte(prop))
				}

			case cc, ok := <-rc.ConfChangeC:
				if !ok {
					rc.ConfChangeC = nil
				} else {
					confChangeCount++
					cc.ID = confChangeCount
					rc.Node.ProposeConfChange(context.TODO(), cc)
				}

			}
		}
		// client closed channel, shutdown raft if not already
		close(rc.Stopc)
	}()

	// event loop on raft state machine updates
	for true {
		select {
		case <-ticker.C:
			rc.Node.Tick()
		// store raft entries to wal, then publish over commit channel
		case rd := <-rc.Node.Ready():
			rc.Wal.Save(rd.HardState, rd.Entries)
			if !raft.IsEmptySnap(rd.Snapshot) {
				rc.saveSnap(rd.Snapshot)
				rc.RaftStorage.ApplySnapshot(rd.Snapshot)
				rc.publishSnapshot(rd.Snapshot)
			}
			rc.RaftStorage.Append(rd.Entries)
			rc.Transport.Send(rd.Messages)
			if ok := rc.publishEntries(rc.entriesToApply(rd.CommittedEntries)); !ok {
				rc.stop()
				return
			}

			rc.maybeTriggerSnapshot()
			rc.Node.Advance()

		case err := <-rc.Transport.ErrorC:
			rc.writeError(err)
			return

		case <-rc.Stopc:
			rc.stop()
			return
		}
	}
}

func (rc *RaftNode) serveRaft() {
	url, err := url.Parse(rc.Peers[rc.Id-1])
	if err != nil {
		log.Fatalf("raft: Failed parsing URL(%v)", err)
	}
	ln, err := newStoppableListener(url.Host, rc.Httpstopc)
	if err != nil {
		log.Fatalf("raft: Failed to listen raftHttp (%v)", err)
	}

	err = (&http.Server{Handler: rc.Transport.Handler()}).Serve(ln)

	select {
	case <-rc.Httpstopc:
	default:
		log.Fatalf("raft: Failed to serve raftHttp (%v)", err)
	}
	close(rc.Httpdonec)
}

func (rc *RaftNode) Process(ctx context.Context, m raftpb.Message) error {
	return rc.Node.Step(ctx, m)
}

func (rc *RaftNode) IsIDRemoved(id uint64) bool {
	return false
}

func (rc *RaftNode) ReportUnreachable(id uint64) {

}

func (rc *RaftNode) ReportSnapshot(id uint64, status raft.SnapshotStatus) {

}
