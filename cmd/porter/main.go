package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

var (
	Date string
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
	configFile := flag.String("config", "./porter.yaml", "porter config file")
	printVersion := flag.Bool("version", true, "print porter version info")
	flag.Parse()

	if *printVersion {
		fmt.Printf("version is %s, build at %s\n",Date,Version)
	}

	fmt.Print(banner)
	fmt.Printf("versiom is %s, build at %s\n",Date,Version)

	if len(*configFile) == 0 {
		fmt.Println("configFile.len error, err: config is nil")
		return
	}
	// build config

	// init

	// start


	// exit func
	sc := make(chan os.Signal, 1)
	signal.Notify(sc,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	go func() {
		for true {
			sig := <- sc
			if sig == syscall.SIGINT || sig == syscall.SIGTERM || sig == syscall.SIGQUIT {
				// stop
				os.Exit(0)
			}
		}
	}()

}