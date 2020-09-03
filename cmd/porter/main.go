package main

import (
	"flag"
	"fmt"
	"github.com/pingcap/errors"
	"os"
	"os/signal"
	"porter/config"
	"porter/server"
	"syscall"
)

var (
	Date    string
	Version string
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
	printVersion := flag.Bool("version", true, "print porter version info")
	flag.Parse()

	if *printVersion {
		fmt.Printf("version is %s, build at %s\n", Date, Version)
	}

	fmt.Print(banner)
	fmt.Printf("versiom is %s, build at %s\n", Date, Version)

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

	go func() {
		for true {
			sig := <-sc
			if sig == syscall.SIGINT || sig == syscall.SIGTERM || sig == syscall.SIGQUIT {
				// stop
				os.Exit(0)
			}
		}
	}()

	s.Run()
}
