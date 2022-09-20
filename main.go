package main

import (
	"flag"
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_server"
	"github.com/near-notfaraway/stevedore/sd_util"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

func main() {
	// parse option
	flag.Usage = showUsage
	daemon := new(bool)
	flag.BoolVar(daemon, "d", false, "Running as a daemon")
	flag.BoolVar(daemon, "daemon", false, "Running as a daemon")
	configPath := new(string)
	flag.StringVar(configPath, "c", "./stevedore.config.json", "Path to config file")
	flag.StringVar(configPath, "config", "./stevedore.config.json", "Path to config file")
	signal := new(string)
	flag.StringVar(signal, "s", "", "Send signal to the process")
	flag.StringVar(signal, "signal", "", "Send signal to the process")
	flag.Parse()
	handleDaemonFlag(*daemon)

	// parse file config
	var config sd_config.Config
	if err := sd_util.UnmarshalFile(*configPath, &config); err != nil {
		panic(fmt.Errorf("init config failed: %w", err))
	}

	// init logger
	cwd(config.Common.WorkingDir)
	if err := sd_util.InitLogger(config.Log); err != nil {
		panic(fmt.Errorf("init logger failed: %w", err))
	}

	// handle signal
	if *signal != "" {
		switch *signal {
		case "kill":
			pid := getPid(config.Common.PidPath)
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
				logrus.Fatalf("send signal for kill failed: %s", err)
			}
			rmPid(config.Common.PidPath)
			logrus.Info("kill stevedore succeed")
			os.Exit(0)

		case "stop":
			pid := getPid(config.Common.PidPath)
			if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
				logrus.Fatalf("send signal for stop failed: %s", err)
			}
			rmPid(config.Common.PidPath)
			logrus.Info("stop stevedore succeed")
			os.Exit(0)

		case "reload":
			pid := getPid(config.Common.PidPath)
			if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
				logrus.Fatalf("send signal for stop failed: %s", err)
			}

		default:
			logrus.Fatalf("signal is invalid: %s", *signal)
		}
	}

	// init worker dir
	if *daemon {
		savePid(config.Common.PidPath)
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
	panic(sd_server.NewServer(&config).ListenAndServe())
}

func handleDaemonFlag(daemon bool) {
	daemonEnv := os.Getenv("STEVEDORE_DAEMON")
	if daemonEnv == "True" {
		return
	}

	if daemon {
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

func getPid(pidFile string) int {
	// use bin dir default
	if pidFile == "" {
		execPath, err := filepath.Abs(os.Args[0])
		if err != nil {
			logrus.Fatalf("get exec path failed: %s", err)
		}
		pidFile = filepath.Join(filepath.Dir(execPath), "stevedore.pid")
	}

	pf, err := os.Open(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return -1
		} else {
			logrus.Fatalf("open pid file failed: %s", err)
		}
	}
	defer pf.Close()

	p, _ := ioutil.ReadAll(pf)
	pid, err := strconv.Atoi(string(p))
	if err != nil {
		logrus.Fatalf("pid in file is invalid: %s", string(p))
	}

	return pid
}

func savePid(pidFile string) {
	// use bin dir default
	if pidFile == "" {
		execPath, err := filepath.Abs(os.Args[0])
		if err != nil {
			logrus.Fatalf("get exec path failed: %s", err)
		}
		pidFile = filepath.Join(filepath.Dir(execPath), "stevedore.pid")
	}

	// get pid and confirm not exist
	if getPid(pidFile) != -1 {
		logrus.Fatal("pid file exist, already running")
	}

	pf, err := os.Create(pidFile)
	if err != nil {
		logrus.Fatal("create pid file failed: %s", err)
	}
	defer pf.Close()

	pid := os.Getpid()
	_, err = pf.Write([]byte(fmt.Sprintf("%d", pid)))
	if err != nil {
		logrus.Fatal("write pid file failed: %s", err)
	}
}

func rmPid(pidFile string) {
	// use bin dir default
	if pidFile == "" {
		execPath, err := filepath.Abs(os.Args[0])
		if err != nil {
			logrus.Fatalf("get exec path failed: %s", err)
		}
		pidFile = filepath.Join(filepath.Dir(execPath), "stevedore.pid")
	}

	if err := os.Remove(pidFile); err != nil {
		logrus.Fatal("remove pid file failed: %s", err)
	}
}

func showUsage() {
	fmt.Printf(`Usage:
    stevedore [options...] [-c <file>] [-s <signal>]	

Options:
    -c/--config <file>      Path to config file
    -d/--daemon             Running as a daemon
    -s/--signal <sig>       Send signal to the process
                            The argument signal can be one of follows:
                              1) kill    SIGKILL
                              2) stop    SIGTERM
                              3) reload  SIGHUP
`,
	)
}
