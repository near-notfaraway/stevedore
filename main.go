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
	if err := config.TestCompletely(); err != nil {
		panic(fmt.Errorf("test config failed: %w", err))
	}

	// cwd and init logger
	cwd(config.Common.WorkingDir)
	if err := sd_diagnosis.InitLogger(config.Log); err != nil {
		panic(fmt.Errorf("init logger failed: %w", err))
	}

	// confirm no pid file
	if pid, err := sd_util.GetPid(config.Common.PidPath); err != nil {
		panic(fmt.Errorf("get pid failed: %w", err))
	} else if pid != -1 {
		logrus.Fatalf("pid exist, stevedore[%d] already running", pid)
	}

	// save pid if as a daemon
	if opt.Daemon {
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
