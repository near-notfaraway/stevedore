package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_diagnosis"
	"github.com/near-notfaraway/stevedore/sd_server"
	"github.com/near-notfaraway/stevedore/sd_util"
	"github.com/sirupsen/logrus"
)

const reloadTimeout = 10 * time.Second

var killProcess = syscall.Kill

// main parses options and delegates all behavior to the unified dispatcher.
func main() {
	if err := runOption(sd_config.ParseOption()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runOption validates one action and dispatches it without hidden process exits.
func runOption(option *sd_config.Option) error {
	action, err := option.Action()
	if err != nil {
		return err
	}
	if action == sd_config.ActionVersion {
		fmt.Println(sd_config.CurrentVersion)
		return nil
	}

	configPath, err := filepath.Abs(option.Config)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	config, err := sd_config.Load(configPath)
	if err != nil {
		return err
	}

	switch action {
	case sd_config.ActionTest:
		if err := sd_config.ValidateConfig(config); err != nil {
			return fmt.Errorf("test config: %w", err)
		}
		if err := sd_server.ValidateRuntimeConfig(config); err != nil {
			return fmt.Errorf("test config: %w", err)
		}
		fmt.Println("test config succeeded")
		return nil
	case sd_config.ActionSignal:
		pid, err := sendSignal(option.Signal, config.Common.PidPath)
		if err != nil {
			return fmt.Errorf("send signal: %w", err)
		}
		fmt.Printf("signal %s sent to stevedore[%d]\n", option.Signal, pid)
		return nil
	case sd_config.ActionRun:
		if err := sd_config.ValidateConfig(config); err != nil {
			return fmt.Errorf("test config: %w", err)
		}
		if err := sd_server.ValidateRuntimeConfig(config); err != nil {
			return fmt.Errorf("test config: %w", err)
		}
		return runServer(configPath, config)
	default:
		return fmt.Errorf("unsupported action %d", action)
	}
}

// runServer initializes process-owned resources and serves until shutdown.
func runServer(configPath string, config *sd_config.Config) error {
	cwd(config.Common.WorkingDir)
	if err := sd_diagnosis.InitLogger(config.Log); err != nil {
		return fmt.Errorf("init logger failed: %w", err)
	}

	pidFile, err := sd_util.AcquirePidFile(config.Common.PidPath)
	if err != nil {
		return fmt.Errorf("acquire pid file failed: %w", err)
	}
	defer func() {
		if err := pidFile.Close(); err != nil {
			logrus.Errorf("close pid file failed: %v", err)
		}
	}()

	if config.PProf.Open {
		go func() {
			if err := http.ListenAndServe(config.PProf.ServerAddr, nil); err != nil {
				panic(fmt.Errorf("open pprof failed: %w", err))
			}
		}()
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT)
	defer signal.Stop(signalCh)
	return serveWithSignals(configPath, config, sd_server.NewServer(config), signalCh)
}

// sendSignal resolves a live process from its PID file and sends one supported signal.
func sendSignal(name, pidPath string) (int, error) {
	var processSignal syscall.Signal
	switch name {
	case "reload":
		processSignal = syscall.SIGHUP
	case "stop":
		processSignal = syscall.SIGTERM
	case "quit":
		processSignal = syscall.SIGQUIT
	default:
		return -1, fmt.Errorf("signal %q is invalid", name)
	}

	pid, err := sd_util.GetRunningPid(pidPath)
	if err != nil {
		return -1, err
	}
	if pid == -1 {
		return -1, fmt.Errorf("stevedore is not running")
	}
	if err := killProcess(pid, processSignal); err != nil {
		return -1, fmt.Errorf("send %s to process %d failed: %w", name, pid, err)
	}
	return pid, nil
}

// serveWithSignals reloads on SIGHUP and waits for clean shutdown on termination signals.
func serveWithSignals(configPath string, current *sd_config.Config, server *sd_server.Server,
	signalCh <-chan os.Signal) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.ListenAndServe(ctx)
	}()

	for {
		select {
		case err := <-serverErrCh:
			return err
		case received := <-signalCh:
			switch received {
			case syscall.SIGHUP:
				reloadCtx, reloadCancel := context.WithTimeout(context.Background(), reloadTimeout)
				next, err := reloadUploadConfig(reloadCtx, configPath, current, server)
				reloadCancel()
				if err != nil {
					logrus.Errorf("reload config failed: %v", err)
					continue
				}
				current = next
				logrus.Info("reload config succeeded")
			case syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT:
				logrus.Infof("received %s, stopping", received)
				cancel()
				return <-serverErrCh
			}
		}
	}
}

// reloadUploadConfig loads, validates, and applies one candidate Upload configuration.
func reloadUploadConfig(ctx context.Context, configPath string, current *sd_config.Config,
	server *sd_server.Server) (*sd_config.Config, error) {
	next, err := sd_config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if err := sd_config.ValidateUploadReload(current, next); err != nil {
		return nil, err
	}
	if err := server.ReloadUpload(ctx, next.Upload); err != nil {
		return nil, err
	}
	return next, nil
}

// cwd changes the process working directory when it differs from the configured path.
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
