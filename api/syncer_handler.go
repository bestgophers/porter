package api

import (
	"encoding/json"
	"github.com/labstack/echo"
	"net/http"
	"porter/config"
	"porter/log"
	"porter/utils"
	"sync"
)

type BinlogSyncerHandler struct {
	l      sync.Mutex
	svr    APIServer
	config config.RaftNodeConfig
}

// StartBinlogSyncer implements start a binlog syncer
func (h *BinlogSyncerHandler) StartBinlogSyncer(echoCtx echo.Context) error {
	h.l.Lock()
	defer h.l.Unlock()

	args := config.SyncerHandleConfig{}

	err := echoCtx.Bind(&args)
	if err != nil {
		return err
	}

	if h.svr.IsLeader() {
		req, err := json.Marshal(args)
		if err != nil {
			return echoCtx.JSON(http.StatusInternalServerError, utils.NewResp().SetError(err.Error()))
		}
		resp, err := h.forwardToLeader("PUT", "startSyncer", req)
		if err != nil {
			log.Log.Errorf("StartBinlog:sendToLeader error,err:%s,args:%v", err, args)
			return echoCtx.JSON(http.StatusInternalServerError, utils.NewResp().SetError(err.Error()))
		}
		if resp.Message != "success" {
			return echoCtx.JSON(http.StatusInternalServerError, utils.NewResp().SetError(resp.Message))
		}
		return echoCtx.JSON(http.StatusOK, utils.NewResp().SetData(resp.Data))
	}
	err = h.svr.StartSyncer(&args)
	if err != nil {
		log.Log.Errorf("StartBinlogServer error,err:%s,args:%v", err, args)
		return err
	}
	return echoCtx.JSON(http.StatusOK, utils.NewResp().SetData(args.ServerID))
}

// StopBinlogSyncer implements stop a binlog syncer
func (h *BinlogSyncerHandler) StopBinlogSyncer(echoCtx echo.Context) error {
	h.l.Lock()
	defer h.l.Unlock()
	args := struct {
		SyncerID uint32 `json:"syncer_id"`
	}{}
	err := echoCtx.Bind(&args)
	if err != nil {
		return err
	}

	if h.svr.IsLeader() == false {
		req, err := json.Marshal(args)
		if err != nil {
			return echoCtx.JSON(http.StatusInternalServerError, utils.NewResp().SetError(err.Error()))
		}

		resp, err := h.forwardToLeader("PUT", "/stopSyncer", req)
		if err != nil {
			log.Log.Errorf("StopBinlogSyncer:sendToLeander error,err:%s,args:%v", err, args)
			return echoCtx.JSON(http.StatusInternalServerError, utils.NewResp().SetError(err.Error()))
		}

		if resp.Message != "success" {
			return echoCtx.JSON(http.StatusInternalServerError, utils.NewResp().SetError(resp.Message))
		}
		return echoCtx.JSON(http.StatusOK, utils.NewResp().SetData(resp.Data))
	}

	h.svr.StopSyncer(args.SyncerID)
	return echoCtx.JSON(http.StatusOK, utils.NewResp().SetData(""))
}

// GetBinlogSyncersStatus returns all syncer status that contains position
func (h *BinlogSyncerHandler) GetBinlogSyncersStatus(echoCtx echo.Context) error {
	h.l.Lock()
	defer h.l.Unlock()

	if h.svr.IsLeader() == false {

		resp, err := h.forwardToLeader("PUT", "/stopSyncer", nil)
		if err != nil {
			log.Log.Errorf("StopBinlogSyncer:sendToLeander error,err:%s,args:%v", err, nil)
			return echoCtx.JSON(http.StatusInternalServerError, utils.NewResp().SetError(err.Error()))
		}

		if resp.Message != "success" {
			return echoCtx.JSON(http.StatusInternalServerError, utils.NewResp().SetError(resp.Message))
		}
		return echoCtx.JSON(http.StatusOK, utils.NewResp().SetData(resp.Data))
	}

	status := h.svr.GetSyncersStatus()
	return echoCtx.JSON(http.StatusOK, utils.NewResp().SetData(status))
}

// UpdateBinlogSyncerConfig implements reset a binlog syncer and restart
func (h *BinlogSyncerHandler) UpdateBinlogSyncerConfig(echoCtx echo.Context) error {
	h.l.Lock()
	defer h.l.Unlock()

	args := config.SyncerHandleConfig{}

	err := echoCtx.Bind(&args)
	if err != nil {
		return err
	}

	if h.svr.IsLeader() {
		req, err := json.Marshal(args)
		if err != nil {
			return echoCtx.JSON(http.StatusInternalServerError, utils.NewResp().SetError(err.Error()))
		}
		resp, err := h.forwardToLeader("PUT", "startSyncer", req)
		if err != nil {
			log.Log.Errorf("StartBinlog:sendToLeader error,err:%s,args:%v", err, args)
			return echoCtx.JSON(http.StatusInternalServerError, utils.NewResp().SetError(err.Error()))
		}
		if resp.Message != "success" {
			return echoCtx.JSON(http.StatusInternalServerError, utils.NewResp().SetError(resp.Message))
		}
		return echoCtx.JSON(http.StatusOK, utils.NewResp().SetData(resp.Data))
	}
	err = h.svr.UpdateSyncer(&args)
	if err != nil {
		log.Log.Errorf("StartBinlogServer error,err:%s,args:%v", err, args)
		return err
	}
	return echoCtx.JSON(http.StatusOK, utils.NewResp().SetData(args.ServerID))
}

// sendToLeander implements forward request to leader in raft cluster
func (h *BinlogSyncerHandler) sendToLeader(method, uri string, req []byte) (*utils.Resp, error) {
	//
	//leaderId := h.svr.Leader()
	//leader := h.cluster.Member(leaderId)
	//if leader == nil {
	//	return nil, ErrNoLeader
	//}
	//
	//if len(leader.AdminURLs) != 1 {
	//	log.Log.Errorf("leader admin url is not 1,leader:%v", *leader)
	//	return nil, ErrNoLeader
	//}
	//leaderURL, err := url.Parse(leader.AdminURLs[0])
	//if err != nil {
	//	return nil, err
	//}
	//reqURL := leaderURL.Scheme + "://" + leaderURL.Host + uri
	//
	//log.Log.Debugf("sendToLeader: reqURL is:%s", req)
	//
	//resp, err := utils.SendRequest(method, reqURL, req)
	//if err != nil {
	//	log.Log.Errorf("sendToLeander: SendRequest error,err:%s,url:%s", err, reqURL)
	//	return nil, err
	//}
	//return resp, nil
	return nil, nil
}

// leaderAddress variable used for cache leader address last invoke
var leaderAddress string

// forwardToLeader implements forward request to the leader node
func (h *BinlogSyncerHandler) forwardToLeader(method, uril string, req []byte) (*utils.Resp, error) {
	// TODO
	return nil, nil
}
