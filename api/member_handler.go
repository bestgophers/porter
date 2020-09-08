package api

import (
	"fmt"
	"github.com/coreos/etcd/pkg/types"
	"github.com/labstack/echo"
	"net/http"
	"net/url"
	"porter/log"
	"porter/raft/member"
	"porter/utils"
	"time"
)

type MemberHandler struct {
	svr     Server
	cluster Cluster
	timeout time.Duration
}

type Cluster interface {
	GetId() string
	Members() []*member.Member
	Member(id types.ID) *member.Member
	MemberByName(name string) *member.Member
}

type Role struct {
	Id string `json:"id"`
	member.RaftAttributes
	member.Attributes
	IsLeader bool `json:"isLeader"`
}

func (h *MemberHandler) GetCluster(echoCtx echo.Context) error {
	members := h.cluster.Members()
	roles := make([]*Role, 0, len(members))
	for _, m := range members {
		r := new(Role)
		r.Id = fmt.Sprintf("%x", uint64(m.Id))
		r.RaftAttributes = m.RaftAttributes
		r.Attributes = m.Attributes
		if uint64(m.Id) == uint64(h.svr.Leader()) {
			r.IsLeader = true
		} else {
			r.IsLeader = false
		}
		roles = append(roles, r)
	}
	return echoCtx.JSON(http.StatusOK, utils.NewResp().SetData(roles))
}

func (h *MemberHandler) GetMembers(echoCtx echo.Context) error {
	members := h.cluster.Members()
	return echoCtx.JSON(http.StatusOK, utils.NewResp().SetData(members))
}

func (h *MemberHandler) SendToLeader(method string, req []byte) (*utils.Resp, error) {
	leaderId := h.svr.Leader()
	leader := h.cluster.Member(leaderId)
	if leader == nil {
		return nil, ErrNoLeader
	}
	if len(leader.AdminURLs) != 1 {
		log.Log.Errorf("leader admin url is not 1,leader:%v", *leader)
		return nil, ErrNoLeader
	}
	leaderUrl, err := url.Parse(leader.AdminURLs[0])
	if err != nil {
		return nil, err
	}
	url := leaderUrl.Scheme + "://" + leaderUrl.Host + "/members"

	resp, err := utils.SendRequest(method, url, req)
	if err != nil {
		log.Log.Errorf("SendToLeader: SendRequest error, err :%s,url%s", err, url)
		return nil, err
	}
	return resp, nil
}
