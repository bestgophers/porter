package member

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"github.com/coreos/etcd/pkg/types"
	"math/rand"
	"porter/log"
	"sort"
	"time"
)

type RaftAttributes struct {
	PeerURLs []string `json:"peerURLs"`
}

type Attributes struct {
	Name      string   `json:"name,omitempty"`
	AdminURLs []string `json:"adminURLs,omitempty"`
}

type Member struct {
	Id types.ID `json:"id"`
	RaftAttributes
	Attributes
}

func NewMember(name string, peerURLs []string, adminURLs []string, now *time.Time) *Member {
	m := &Member{
		RaftAttributes: RaftAttributes{PeerURLs: peerURLs},
		Attributes:     Attributes{Name: name, AdminURLs: adminURLs},
	}
	var b []byte

	b = append(b, []byte(m.Attributes.Name)...)

	sort.Strings(m.PeerURLs)

	for _, p := range m.PeerURLs {
		b = append(b, []byte(p)...)
	}

	if now != nil {
		b = append(b, []byte(fmt.Sprintf("%d", now.Unix()))...)
	}
	hash := sha1.Sum(b)
	m.Id = types.ID(binary.BigEndian.Uint64(hash[:8]))
	return m
}

func (m *Member) PickPeerURL() string {
	if len(m.PeerURLs) == 0 {
		log.Log.Panicf("member should always have some peer url")
	}
	return m.PeerURLs[rand.Intn(len(m.PeerURLs))]
}

func (m *Member) Clone() *Member {
	if m == nil {
		return nil
	}
	mm := &Member{
		Id:         m.Id,
		Attributes: Attributes{Name: m.Name},
	}
	if m.PeerURLs != nil {
		mm.PeerURLs = make([]string, len(m.PeerURLs))
		copy(mm.PeerURLs, m.PeerURLs)
	}

	if m.AdminURLs != nil {
		mm.AdminURLs = make([]string, len(m.AdminURLs))
		copy(mm.AdminURLs, m.AdminURLs)
	}
	return mm
}

func (m *Member) IsStarted() bool {
	return len(m.Name) != 0
}
