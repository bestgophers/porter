package api

import (
	"context"
	"github.com/coreos/etcd/pkg/types"
	"github.com/labstack/echo"
	mw "github.com/labstack/echo/middleware"
	"porter/config"
	"porter/log"
	"time"
)

type APIServer interface {
	Leader() types.ID

	IsLeader() bool

	StopSyncer(syncerId uint32)

	StartSyncer(cfg *config.SyncerHandleConfig) error

	UpdateSyncer(cfg *config.SyncerHandleConfig) error

	GetSyncersStatus() interface{}
}

type AdminServer struct {
	AdminAddr string
	web       *echo.Echo
	bs        *BinlogSyncerHandler
}

func NewAdminServer(addr string, svr APIServer) *AdminServer {
	return &AdminServer{
		AdminAddr: addr,
		web:       echo.New(),
		bs: &BinlogSyncerHandler{
			svr: svr,
		},
	}
}

func (s *AdminServer) Run() {
	s.RegisterMiddleware()
	s.RegisterURL()
	err := s.web.Start(s.AdminAddr)
	if err != nil {
		log.Log.Infof("admin server start error,err: %s", err)
	}
}

//RegisterMiddleware implements register middleware in web
func (s *AdminServer) RegisterMiddleware() {
	loggerConfig := mw.LoggerConfig{
		Skipper: mw.DefaultSkipper,
		Format: `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}","host":"${host}",` +
			`"method":"${method}","uri":"${uri}","status":${status}, "latency":${latency},` +
			`"latency_human":"${latency_human}","bytes_in":${bytes_in},` +
			`"bytes_out":${bytes_out}}` + "\n",
		CustomTimeFormat: "2006-01-02 15:04:05.00000",
		Output:           log.NewWriter(),
	}
	s.web.Use(mw.LoggerWithConfig(loggerConfig))
	s.web.Use(mw.Recover())
}

func (s *AdminServer) RegisterURL() {
	s.web.GET("/syncers", s.bs.GetBinlogSyncersStatus)
	s.web.POST("/startSyncer", s.bs.StartBinlogSyncer)
	s.web.PUT("/updateSyncer", s.bs.UpdateBinlogSyncerConfig)
	s.web.GET("/stopSyncer", s.bs.StopBinlogSyncer)
	s.web.GET("/isLeader", s.bs.IsLeader)
}

func (s *AdminServer) Stop() {
	ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFunc()
	if err := s.web.Shutdown(ctx); err != nil {
		log.Log.Fatalf("adminServer Shutdown error:%s", err.Error())
	}
}
