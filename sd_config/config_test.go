package sd_config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestLoadConvey(t *testing.T) {
	convey.Convey("Test Load", t, func() {
		convey.Convey("returns independent configs", func() {
			configPath := filepath.Join(t.TempDir(), "stevedore.json")
			convey.So(os.WriteFile(configPath, []byte(`{
		"Common": {
			"WorkingDir": "/first",
			"PidPath": "/first/stevedore.pid"
		}
	}`), 0o600), convey.ShouldBeNil)

			first, err := Load(configPath)
			convey.So(err, convey.ShouldBeNil)

			convey.So(os.WriteFile(configPath, []byte(`{
		"Common": {
			"WorkingDir": "/second",
			"PidPath": "/second/stevedore.pid"
		}
	}`), 0o600), convey.ShouldBeNil)
			second, err := Load(configPath)
			convey.So(err, convey.ShouldBeNil)

			convey.So(first, convey.ShouldNotEqual, second)
			convey.So(first.Common.WorkingDir, convey.ShouldEqual, "/first")
			convey.So(second.Common.WorkingDir, convey.ShouldEqual, "/second")
		})

		convey.Convey("rejects missing common config", func() {
			configPath := filepath.Join(t.TempDir(), "stevedore.json")
			convey.So(os.WriteFile(configPath, []byte(`{}`), 0o600), convey.ShouldBeNil)

			config, err := Load(configPath)

			convey.So(config, convey.ShouldBeNil)
			convey.So(err, convey.ShouldResemble, fmt.Errorf("common config is required"))
		})

		convey.Convey("returns unmarshal errors", func() {
			configPath := filepath.Join(t.TempDir(), "stevedore.json")
			convey.So(os.WriteFile(configPath, []byte(`{`), 0o600), convey.ShouldBeNil)

			config, err := Load(configPath)

			convey.So(config, convey.ShouldBeNil)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "load config")
		})
	})
}

func TestConfigApplyCommonDefaultsConvey(t *testing.T) {
	convey.Convey("Test Config.applyCommonDefaults", t, func() {
		convey.Convey("fills empty common paths from the executable path", func() {
			config := &Config{Common: &CommonConfig{}}
			execPath, err := filepath.Abs(os.Args[0])
			convey.So(err, convey.ShouldBeNil)

			convey.So(config.applyCommonDefaults(), convey.ShouldBeNil)
			convey.So(config.Common.WorkingDir, convey.ShouldEqual, filepath.Dir(filepath.Dir(execPath)))
			convey.So(config.Common.PidPath, convey.ShouldEqual,
				filepath.Join(filepath.Dir(execPath), "stevedore.pid"))
		})

		convey.Convey("preserves explicit common paths", func() {
			config := &Config{Common: &CommonConfig{
				WorkingDir: "/work",
				PidPath:    "/run/stevedore.pid",
			}}

			convey.So(config.applyCommonDefaults(), convey.ShouldBeNil)
			convey.So(config.Common.WorkingDir, convey.ShouldEqual, "/work")
			convey.So(config.Common.PidPath, convey.ShouldEqual, "/run/stevedore.pid")
		})
	})
}

func TestValidateConfigConvey(t *testing.T) {
	convey.Convey("Test ValidateConfig", t, func() {
		convey.Convey("accepts a valid config", func() {
			convey.So(ValidateConfig(validConfig()), convey.ShouldBeNil)
		})

		convey.Convey("rejects a nil config", func() {
			convey.So(ValidateConfig(nil), convey.ShouldResemble, fmt.Errorf("config is required"))
		})

		missingSections := []struct {
			name   string
			remove func(*Config)
		}{
			{name: "Common", remove: func(config *Config) { config.Common = nil }},
			{name: "PProf", remove: func(config *Config) { config.PProf = nil }},
			{name: "Log", remove: func(config *Config) { config.Log = nil }},
			{name: "Server", remove: func(config *Config) { config.Server = nil }},
			{name: "Upload", remove: func(config *Config) { config.Upload = nil }},
			{name: "Session", remove: func(config *Config) { config.Session = nil }},
		}
		for _, test := range missingSections {
			convey.Convey("rejects missing "+test.name, func() {
				config := validConfig()
				test.remove(config)

				convey.So(ValidateConfig(config), convey.ShouldResemble,
					fmt.Errorf("%s config is required", test.name))
			})
		}

		invalidFields := []struct {
			name      string
			change    func(*Config)
			errorText string
		}{
			{name: "log level", change: func(config *Config) { config.Log.Level = "invalid" },
				errorText: "log level is invalid"},
			{name: "log path", change: func(config *Config) { config.Log.Path = "" },
				errorText: "log path is required"},
			{name: "log max age", change: func(config *Config) { config.Log.MaxAgeHour = 0 },
				errorText: "log max age must be positive"},
			{name: "log rotation time", change: func(config *Config) { config.Log.RotationTimeHour = 0 },
				errorText: "log rotation time must be positive"},
			{name: "pprof address", change: func(config *Config) {
				config.PProf.Open = true
				config.PProf.ServerAddr = "invalid:::"
			}, errorText: "pprof server address is invalid"},
			{name: "server max tries", change: func(config *Config) { config.Server.MaxTryTimes = 0 },
				errorText: "server max try times must be positive"},
			{name: "session recycle interval", change: func(config *Config) {
				config.Session.RecycleIntervalSec = 0
			}, errorText: "session recycle interval must be positive"},
			{name: "session timeout", change: func(config *Config) { config.Session.TimeoutSec = -1 },
				errorText: "session timeout must not be negative"},
		}
		for _, test := range invalidFields {
			convey.Convey("rejects invalid "+test.name, func() {
				config := validConfig()
				test.change(config)

				err := ValidateConfig(config)
				convey.So(err, convey.ShouldNotBeNil)
				convey.So(err.Error(), convey.ShouldContainSubstring, test.errorText)
			})
		}
	})
}

func validConfig() *Config {
	return &Config{
		Common: &CommonConfig{
			WorkingDir: "/tmp",
			PidPath:    "/tmp/stevedore.pid",
		},
		PProf: &PProfConfig{},
		Log: &LogConfig{
			Path:             "/tmp/stevedore.log",
			Level:            "info",
			MaxAgeHour:       24,
			RotationTimeHour: 24,
		},
		Server: &ServerConfig{
			ListenAddr:  "127.0.0.1:2614",
			MaxTryTimes: 3,
		},
		Upload: &UploadConfig{},
		Session: &SessionConfig{
			RecycleIntervalSec: 10,
			TimeoutSec:         30,
		},
	}
}

func TestValidateUploadReloadConvey(t *testing.T) {
	convey.Convey("Test ValidateUploadReload", t, func() {
		convey.Convey("allows upload changes", func() {
			current := testReloadConfig()
			next := testReloadConfig()
			next.Upload = &UploadConfig{DefaultUpstream: "reloaded"}

			convey.So(ValidateUploadReload(current, next), convey.ShouldBeNil)
		})

		tests := map[string]func(*Config){
			"Common":  func(config *Config) { config.Common.WorkingDir = "/changed" },
			"PProf":   func(config *Config) { config.PProf.Open = true },
			"Log":     func(config *Config) { config.Log.Level = "debug" },
			"Server":  func(config *Config) { config.Server.BatchSize++ },
			"Session": func(config *Config) { config.Session.TimeoutSec++ },
		}
		for section, change := range tests {
			convey.Convey("rejects non-upload changes to "+section, func() {
				current := testReloadConfig()
				next := testReloadConfig()
				change(next)

				err := ValidateUploadReload(current, next)

				convey.So(err, convey.ShouldResemble,
					fmt.Errorf("non-Upload config changed: %s", section))
			})
		}
	})
}

func testReloadConfig() *Config {
	return &Config{
		Common:  &CommonConfig{WorkingDir: "/work", PidPath: "/run/stevedore.pid"},
		PProf:   &PProfConfig{ServerAddr: ":6060"},
		Log:     &LogConfig{Level: "info"},
		Server:  &ServerConfig{ListenAddr: ":2614", BatchSize: 32},
		Upload:  &UploadConfig{DefaultUpstream: "default"},
		Session: &SessionConfig{TimeoutSec: 30},
	}
}

func TestRouteConfigJSONConvey(t *testing.T) {
	convey.Convey("Test RouteConfig JSON contract", t, func() {
		convey.Convey("uses Upstream as the only routing target", func() {
			raw := []byte(`{
		"DefaultAction": "drop",
		"DefaultUpstream": "drop",
		"Routes": [{
			"Name": "canary",
			"Action": "drop",
			"Upstream": "drop",
			"KeyBytes": "1:2",
			"Operator": "!==",
			"Value": "0x01",
			"Mask": "0x80",
			"Conditions": [{
				"KeyBytes": "0:1",
				"Operator": "==",
				"Value": "0x80",
				"Mask": "0x80"
			}]
		}]
	}`)
			var config UploadConfig
			convey.So(json.Unmarshal(raw, &config), convey.ShouldBeNil)

			encoded, err := json.Marshal(config)
			convey.So(err, convey.ShouldBeNil)
			var decoded map[string]any
			convey.So(json.Unmarshal(encoded, &decoded), convey.ShouldBeNil)
			convey.So(decoded, convey.ShouldNotContainKey, "DefaultAction")
			convey.So(decoded["DefaultUpstream"], convey.ShouldEqual, "drop")

			routes, ok := decoded["Routes"].([]any)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(routes, convey.ShouldHaveLength, 1)
			route, ok := routes[0].(map[string]any)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(route["Name"], convey.ShouldEqual, "canary")
			convey.So(route, convey.ShouldNotContainKey, "Action")
			convey.So(route["Upstream"], convey.ShouldEqual, "drop")
			convey.So(route, convey.ShouldNotContainKey, "KeyBytes")
			convey.So(route, convey.ShouldNotContainKey, "Operator")
			convey.So(route, convey.ShouldNotContainKey, "Value")
			convey.So(route, convey.ShouldNotContainKey, "Mask")

			conditions, ok := route["Conditions"].([]any)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(conditions, convey.ShouldHaveLength, 1)
			condition, ok := conditions[0].(map[string]any)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(condition["Mask"], convey.ShouldEqual, "0x80")
		})
	})
}

func TestLogConfigJSONConvey(t *testing.T) {
	convey.Convey("Test LogConfig JSON contract", t, func() {
		convey.Convey("exposes upload stats interval", func() {
			raw := []byte(`{
				"Log": {
					"UploadStatsIntervalSec": 60
				}
			}`)
			var config Config
			convey.So(json.Unmarshal(raw, &config), convey.ShouldBeNil)

			encoded, err := json.Marshal(config)
			convey.So(err, convey.ShouldBeNil)
			var decoded map[string]any
			convey.So(json.Unmarshal(encoded, &decoded), convey.ShouldBeNil)

			logConfig := decoded["Log"].(map[string]any)
			convey.So(logConfig["UploadStatsIntervalSec"], convey.ShouldEqual, float64(60))
			convey.So(logConfig, convey.ShouldNotContainKey, "RouteStatsIntervalSec")
		})
	})
}

func TestUpstreamConfigJSONConvey(t *testing.T) {
	convey.Convey("Test UpstreamConfig JSON contract", t, func() {
		convey.Convey("nests fixed key under extractor", func() {
			config := UpstreamConfig{
				Name: "fixed",
				Type: "chash",
				KeyExtractor: &KeyExtractorConfig{
					Type: "fixed",
					Fixed: &FixedKeyConfig{
						KeyBytes: "0:2,4:6",
						KeyMask:  "0xffffffff",
					},
				},
			}

			encoded, err := json.Marshal(config)
			convey.So(err, convey.ShouldBeNil)
			var decoded map[string]any
			convey.So(json.Unmarshal(encoded, &decoded), convey.ShouldBeNil)

			convey.So(decoded, convey.ShouldNotContainKey, "KeyBytes")
			convey.So(decoded, convey.ShouldNotContainKey, "KeyMask")
			extractor := decoded["KeyExtractor"].(map[string]any)
			convey.So(extractor["Type"], convey.ShouldEqual, "fixed")
			fixed := extractor["Fixed"].(map[string]any)
			convey.So(fixed["KeyBytes"], convey.ShouldEqual, "0:2,4:6")
			convey.So(fixed["KeyMask"], convey.ShouldEqual, "0xffffffff")
		})
	})
}
