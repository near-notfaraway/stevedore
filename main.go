package main

import (
	"flag"
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_server"
	"github.com/near-notfaraway/stevedore/sd_util"
	"net/http"
	"os"
	"os/exec"
)

func main() {
	// parse option
	daemon := flag.Bool("daemon", false, "Running as a daemon")
	configPath := flag.String("config", "../etc/config.json", "Path to config file")
	flag.Parse()
	handleDaemonFlag(daemon)

	// parse file config
	var config sd_config.Config
	if err := sd_util.UnmarshalFile(*configPath, &config); err != nil {
		panic(fmt.Errorf("init config failed: %w", err))
	}

	// init pprof
	if config.PProf.Open {
		go func() {
			if err := http.ListenAndServe(config.PProf.ServerAddr, nil); err != nil {
				panic(fmt.Errorf("open pprof failed: %w", err))
			}
		}()
	}

	// init logger
	if err := sd_util.InitLogger(config.Log); err != nil {
		panic(fmt.Errorf("init logger failed: %w", err))
	}

	// listen and serve
	panic(sd_server.NewServer(&config).ListenAndServe())
}


func handleDaemonFlag(daemon *bool) {
	daemonEnv := os.Getenv("STEVEDORE_DAEMON")
	if daemonEnv == "True" {
		return
	}

	if *daemon {
		cmd := exec.Command(os.Args[0], os.Args[1:]...)
		cmd.Env = []string{"STEVEDORE_DAEMON=True"}
		if err := cmd.Start(); err != nil {
			fmt.Printf("start %s failed, error: %v\n", os.Args[0], err)
			os.Exit(1)
		}
		fmt.Printf("%s [PID] %d running...\n", os.Args[0], cmd.Process.Pid)
		os.Exit(0)
	}
}
