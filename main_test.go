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
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestServeWithSignalsConvey(t *testing.T) {
	convey.Convey("Test serveWithSignals", t, func() {
		convey.Convey("reloads upload config on SIGHUP", func() {
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
			convey.So(err, convey.ShouldBeNil)
			configPath := filepath.Join(t.TempDir(), "stevedore.json")
			convey.So(os.WriteFile(configPath, raw, 0o600), convey.ShouldBeNil)

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
				convey.So(upload.DefaultUpstream, convey.ShouldEqual, "drop")
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for reload")
			}

			stopErr := errors.New("server stopped")
			serveErr <- stopErr
			convey.So(errors.Is(<-done, stopErr), convey.ShouldBeTrue)
		})

		for _, processSignal := range []os.Signal{syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT} {
			convey.Convey("stops on "+processSignal.String(), func() {
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
					convey.So(err, convey.ShouldBeNil)
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for signal shutdown")
				}
			})
		}
	})
}

func TestReloadUploadConfigConvey(t *testing.T) {
	convey.Convey("Test reloadUploadConfig", t, func() {
		convey.Convey("rejects static config changes", func() {
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
			convey.So(err, convey.ShouldBeNil)
			configPath := filepath.Join(t.TempDir(), "stevedore.json")
			convey.So(os.WriteFile(configPath, raw, 0o600), convey.ShouldBeNil)
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

			convey.So(reloaded, convey.ShouldBeNil)
			convey.So(err, convey.ShouldResemble, errors.New("non-Upload config changed: Server"))
			convey.So(reloadCalled, convey.ShouldBeFalse)
		})
	})
}

func TestRunOptionConvey(t *testing.T) {
	convey.Convey("Test runOption", t, func() {
		convey.Convey("rejects invalid basic config before runtime validation", func() {
			config := validMainConfig(t.TempDir(), filepath.Join(t.TempDir(), "stevedore.pid"))
			config.Log.Level = "invalid"
			configPath := writeMainConfig(t, config)

			err := runOption(&sd_config.Option{Config: configPath, Test: true})

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "test config: log level is invalid")
		})

		convey.Convey("rejects invalid runtime config", func() {
			config := validMainConfig(t.TempDir(), filepath.Join(t.TempDir(), "stevedore.pid"))
			config.Server.ListenAddr = "invalid"
			configPath := writeMainConfig(t, config)

			err := runOption(&sd_config.Option{Config: configPath, Test: true})

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "test config: server listen address")
		})

		convey.Convey("accepts valid config in test mode", func() {
			config := validMainConfig(t.TempDir(), filepath.Join(t.TempDir(), "stevedore.pid"))
			configPath := writeMainConfig(t, config)

			convey.So(runOption(&sd_config.Option{Config: configPath, Test: true}), convey.ShouldBeNil)
		})

		convey.Convey("loads valid config and starts server", func() {
			configPath := filepath.Join(t.TempDir(), "stevedore.json")
			workingDir, err := os.Getwd()
			convey.So(err, convey.ShouldBeNil)
			server := &sd_server.Server{}
			stopErr := errors.New("mock server stopped")
			loggerCalled := false
			config := validMainConfig(workingDir, filepath.Join(t.TempDir(), "stevedore.pid"))
			raw, err := json.Marshal(config)
			convey.So(err, convey.ShouldBeNil)
			convey.So(os.WriteFile(configPath, raw, 0o600), convey.ShouldBeNil)

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

			convey.So(errors.Is(err, stopErr), convey.ShouldBeTrue)
			convey.So(loggerCalled, convey.ShouldBeTrue)
		})
	})
}

func TestSendSignalConvey(t *testing.T) {
	convey.Convey("Test sendSignal", t, func() {
		convey.Convey("signals the running pid without removing its file", func() {
			pidPath := filepath.Join(t.TempDir(), "stevedore.pid")
			owner, err := sd_util.AcquirePidFile(pidPath)
			convey.So(err, convey.ShouldBeNil)
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

			convey.So(err, convey.ShouldBeNil)
			convey.So(pid, convey.ShouldEqual, os.Getpid())
			convey.So(gotPID, convey.ShouldEqual, os.Getpid())
			convey.So(gotSignal, convey.ShouldEqual, syscall.SIGQUIT)
			_, err = os.Stat(pidPath)
			convey.So(err, convey.ShouldBeNil)
		})
	})
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

func writeMainConfig(t *testing.T, config *sd_config.Config) string {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "stevedore.json")
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, raw, 0o600))
	return configPath
}
