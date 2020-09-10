package main

import (
	"flag"
	"fmt"
	"github.com/pingcap/errors"
	"os"
	"os/signal"
	"porter/config"
	"porter/log"
	"porter/server"
	"syscall"
)

const banner string = `
______          _            
| ___ \        | |           
| |_/ /__  _ __| |_ ___ _ __ 
|  __/ _ \| '__| __/ _ \ '__|
| | | (_) | |  | ||  __/ |   
\_|  \___/|_|   \__\___|_|
`

func main() {
	configFile := flag.String("config", "./etc/porter.toml", "porter config file")
	flag.Parse()

	fmt.Print(banner)

	if len(*configFile) == 0 {
		fmt.Println("configFile.len error, err: config is nil")
		return
	}
	// build config
	porterConfig, err := config.NewPorterConfig(*configFile)
	if err != nil {
		fmt.Printf("NewPorterConfig error, err:%s\n", err.Error())
		return
	}

	// init log
	log.InitLoggers(porterConfig.LogDir, porterConfig.LogLevel)
	defer log.UnInitLoggers()

	// start
	s, err := server.NewServer(porterConfig)
	if err != nil {
		println(errors.ErrorStack(err))
		return
	}

	// exit func
	sc := make(chan os.Signal, 1)
	signal.Notify(sc,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	done := make(chan struct{}, 1)
	go func() {
		s.Run()
		done <- struct{}{}
	}()

	select {
	case n := <-sc:
		log.Log.Infof("receive signal %v, closing", n)
	case <-s.Ctx().Done():
		log.Log.Infof("context is done with %v, closing", s.Ctx().Err())
	}

	s.Close()
	<-done
}
