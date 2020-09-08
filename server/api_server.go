package server

import (
	"context"
	"github.com/labstack/echo"
	mw "github.com/labstack/echo/middleware"
	"porter/log"
	"time"
)

const (
	adminAPITimeout = 10 * time.Second
)

type AdminServer struct {
	AdminAddr string
	web       *echo.Echo
	sy        *Server
}

type ApiServer interface {
	AddSyncer()
	RemoveSyncer()
	UpdateSyncer()
	AllSyncers()

	StopServer()
}

func NewAdminServer(addr string, svr ApiServer) *AdminServer {
	return &AdminServer{
		AdminAddr: addr,
		web:       echo.New(),
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
	s.web.GET("/allSyncers", s.sy.AllSyncers)
	s.web.POST("/addSyncer", s.sy.AddCanal)
	s.web.PUT("/updateSyncer", s.sy.ResetCanal)
	s.web.GET("/removeSyncer", s.sy.RemoveCanal)
	s.web.GET("/stopServer", s.sy.StopCanal)
}

func (s *AdminServer) Stop() {
	ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFunc()
	if err := s.web.Shutdown(ctx); err != nil {
		log.Log.Fatalf("adminServer Shutdown error:%s", err.Error())
	}
}
