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

// IsLeader returns determine whether the current node is leader
func (h *BinlogSyncerHandler) IsLeader(echoCtx echo.Context) error {
	leader := h.svr.IsLeader()
	return echoCtx.JSON(http.StatusOK, utils.NewResp().SetData(leader))
}

// leaderAddress variable used for cache leader address last invoke
var leaderAddress string

// forwardToLeader implements forward request to the leader node
func (h *BinlogSyncerHandler) forwardToLeader(method, url string, req []byte) (*utils.Resp, error) {
	var found bool

	if len(leaderAddress) > 0 {
		resp, err := utils.SendRequest("GET", leaderAddress+"/isLeader", nil)
		if err == nil && resp.Data.(bool) {
			found = true
		} else {
			for _, peer := range h.config.Peers {
				reqURL := "http://" + peer
				if reqURL == leaderAddress {
					continue
				}
				resp, err := utils.SendRequest("GET", reqURL+"/isLeader", nil)
				if err == nil && resp.Data.(bool) {
					found = true
					leaderAddress = reqURL
					break
				}
			}
		}
	}

	if found {
		resp, err := utils.SendRequest(method, leaderAddress+url, req)
		if err != nil {
			log.Log.Errorf("forwardToLeader: sendRequest error, err: %s", err.Error())
			return nil, err
		}
		return resp, nil
	}
	return nil, nil
}
