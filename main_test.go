package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_diagnosis"
	"github.com/near-notfaraway/stevedore/sd_server"
	"github.com/near-notfaraway/stevedore/sd_util"
	"github.com/stretchr/testify/require"
)

func TestServeWithReloadHandlesSIGHUP(t *testing.T) {
	current := &sd_config.Config{
		Common:  &sd_config.CommonConfig{WorkingDir: "/work", PidPath: "/run/stevedore.pid"},
		PProf:   &sd_config.PProfConfig{ServerAddr: ":6060"},
		Log:     &sd_config.LogConfig{Level: "info"},
		Server:  &sd_config.ServerConfig{ListenAddr: ":2614", BatchSize: 32},
		Upload:  &sd_config.UploadConfig{DefaultUpstream: "old"},
		Session: &sd_config.SessionConfig{TimeoutSec: 30},
	}
	next := *current
	next.Upload = &sd_config.UploadConfig{DefaultUpstream: "drop"}
	raw, err := json.Marshal(&next)
	require.NoError(t, err)
	configPath := filepath.Join(t.TempDir(), "stevedore.json")
	require.NoError(t, os.WriteFile(configPath, raw, 0o600))

	server := &sd_server.Server{}
	reloads := make(chan *sd_config.UploadConfig, 1)
	serveErr := make(chan error, 1)
	patches := gomonkey.NewPatches()
	patches.ApplyMethod(server, "ListenAndServe", func(_ *sd_server.Server, ctx context.Context) error {
		select {
		case err := <-serveErr:
			return err
		case <-ctx.Done():
			return nil
		}
	})
	patches.ApplyMethod(server, "ReloadUpload",
		func(_ *sd_server.Server, _ context.Context, config *sd_config.UploadConfig) error {
			reloads <- config
			return nil
		},
	)
	defer patches.Reset()
	reloadCh := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- serveWithSignals(configPath, current, server, reloadCh)
	}()

	reloadCh <- syscall.SIGHUP
	select {
	case upload := <-reloads:
		require.Equal(t, "drop", upload.DefaultUpstream)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reload")
	}

	stopErr := errors.New("server stopped")
	serveErr <- stopErr
	require.ErrorIs(t, <-done, stopErr)
}

func TestServeWithSignalsStopsOnTerminationSignals(t *testing.T) {
	for _, processSignal := range []os.Signal{syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT} {
		t.Run(processSignal.String(), func(t *testing.T) {
			server := &sd_server.Server{}
			patches := gomonkey.ApplyMethod(server, "ListenAndServe",
				func(_ *sd_server.Server, ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
			)
			defer patches.Reset()
			signalCh := make(chan os.Signal, 1)
			done := make(chan error, 1)
			go func() {
				done <- serveWithSignals("unused.json", &sd_config.Config{}, server, signalCh)
			}()

			signalCh <- processSignal

			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for signal shutdown")
			}
		})
	}
}

func TestReloadUploadConfigRejectsStaticChanges(t *testing.T) {
	current := &sd_config.Config{
		Common:  &sd_config.CommonConfig{WorkingDir: "/work", PidPath: "/run/stevedore.pid"},
		PProf:   &sd_config.PProfConfig{ServerAddr: ":6060"},
		Log:     &sd_config.LogConfig{Level: "info"},
		Server:  &sd_config.ServerConfig{ListenAddr: ":2614", BatchSize: 32},
		Upload:  &sd_config.UploadConfig{DefaultUpstream: "old"},
		Session: &sd_config.SessionConfig{TimeoutSec: 30},
	}
	next := *current
	next.Server = &sd_config.ServerConfig{ListenAddr: ":2614", BatchSize: 64}
	raw, err := json.Marshal(&next)
	require.NoError(t, err)
	configPath := filepath.Join(t.TempDir(), "stevedore.json")
	require.NoError(t, os.WriteFile(configPath, raw, 0o600))
	server := &sd_server.Server{}
	reloadCalled := false
	patches := gomonkey.ApplyMethod(server, "ReloadUpload",
		func(_ *sd_server.Server, _ context.Context, _ *sd_config.UploadConfig) error {
			reloadCalled = true
			return nil
		},
	)
	defer patches.Reset()

	reloaded, err := reloadUploadConfig(context.Background(), configPath, current, server)

	require.Nil(t, reloaded)
	require.EqualError(t, err, "non-Upload config changed: Server")
	require.False(t, reloadCalled)
}

func TestRunOptionLoadsConfigAndStartsServer(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "stevedore.json")
	workingDir, err := os.Getwd()
	require.NoError(t, err)
	server := &sd_server.Server{}
	stopErr := errors.New("mock server stopped")
	loggerCalled := false
	config := validMainConfig(workingDir, filepath.Join(t.TempDir(), "stevedore.pid"))
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, raw, 0o600))

	patches := gomonkey.NewPatches()
	patches.ApplyFunc(
		sd_diagnosis.InitLogger,
		func(*sd_config.LogConfig) error {
			loggerCalled = true
			return nil
		},
	)
	patches.ApplyFuncReturn(sd_server.NewServer, server)
	patches.ApplyMethodReturn(server, "ListenAndServe", stopErr)
	defer patches.Reset()

	err = runOption(&sd_config.Option{Config: configPath})

	require.ErrorIs(t, err, stopErr)
	require.True(t, loggerCalled)
}

func TestSendSignalUsesRunningPidWithoutRemovingFile(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "stevedore.pid")
	owner, err := sd_util.AcquirePidFile(pidPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = owner.Close()
	})
	previousKillProcess := killProcess
	var gotPID int
	var gotSignal syscall.Signal
	killProcess = func(pid int, signal syscall.Signal) error {
		gotPID = pid
		gotSignal = signal
		return nil
	}
	t.Cleanup(func() {
		killProcess = previousKillProcess
	})

	pid, err := sendSignal("quit", pidPath)

	require.NoError(t, err)
	require.Equal(t, os.Getpid(), pid)
	require.Equal(t, os.Getpid(), gotPID)
	require.Equal(t, syscall.SIGQUIT, gotSignal)
	_, err = os.Stat(pidPath)
	require.NoError(t, err)
}

func validMainConfig(workingDir, pidPath string) *sd_config.Config {
	return &sd_config.Config{
		Common: &sd_config.CommonConfig{WorkingDir: workingDir, PidPath: pidPath},
		PProf:  &sd_config.PProfConfig{},
		Log: &sd_config.LogConfig{
			Path:             filepath.Join(workingDir, "stevedore.log"),
			Level:            "info",
			MaxAgeHour:       24,
			RotationTimeHour: 24,
		},
		Server: &sd_config.ServerConfig{
			ListenAddr:  "127.0.0.1:2614",
			MaxTryTimes: 3,
		},
		Upload: &sd_config.UploadConfig{DefaultUpstream: "drop"},
		Session: &sd_config.SessionConfig{
			RecycleIntervalSec: 1,
			TimeoutSec:         30,
		},
	}
}
