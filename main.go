package main

import (
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_diagnosis"
	"github.com/near-notfaraway/stevedore/sd_server"
	"github.com/near-notfaraway/stevedore/sd_util"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	// parse and handle option
	opt := sd_config.ParseOption()
	opt.Handle()

	// test config
	config := sd_config.GlobalConfig
	if err := config.TestAndComplete(); err != nil {
		panic(fmt.Errorf("test config failed: %w", err))
	}

	// init logger
	cwd(config.Common.WorkingDir)
	if err := sd_diagnosis.InitLogger(config.Log); err != nil {
		panic(fmt.Errorf("init logger failed: %w", err))
	}

	// save pid if as a daemon
	if opt.Daemon {
		if pid, _ := sd_util.GetPid(config.Common.PidPath); pid != -1 {
			panic("stevedore is not running")
		}
		if err := sd_util.SavePid(config.Common.PidPath); err != nil {
			panic(fmt.Errorf("save pid failed: %w", err))
		}
	}

	// init pprof
	if config.PProf.Open {
		go func() {
			if err := http.ListenAndServe(config.PProf.ServerAddr, nil); err != nil {
				panic(fmt.Errorf("open pprof failed: %w", err))
			}
		}()
	}

	// listen and serve
	panic(sd_server.NewServer(config).ListenAndServe())
}

func cwd(wd string) {
	// use project dir default
	if wd == "" {
		execPath, err := filepath.Abs(os.Args[0])
		if err != nil {
			logrus.Fatalf("get exec path failed: %s", err)
		}
		wd = filepath.Dir(filepath.Dir(execPath))
	}

	pwd, _ := os.Getwd()
	if pwd == wd {
		logrus.Infof("working dir is: %s", wd)
		return
	}

	if err := os.Chdir(wd); err != nil {
		logrus.Fatalf("cwd failed: %s", err)
	}
	logrus.Infof("working dir is: %s", wd)
}
