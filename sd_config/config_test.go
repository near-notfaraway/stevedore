package sd_config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadReturnsIndependentConfigs(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "stevedore.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{
		"Common": {
			"WorkingDir": "/first",
			"PidPath": "/first/stevedore.pid"
		}
	}`), 0o600))

	first, err := Load(configPath)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(configPath, []byte(`{
		"Common": {
			"WorkingDir": "/second",
			"PidPath": "/second/stevedore.pid"
		}
	}`), 0o600))
	second, err := Load(configPath)
	require.NoError(t, err)

	require.NotSame(t, first, second)
	require.Equal(t, "/first", first.Common.WorkingDir)
	require.Equal(t, "/second", second.Common.WorkingDir)
}

func TestLoadRejectsMissingCommonConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "stevedore.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{}`), 0o600))

	config, err := Load(configPath)

	require.Nil(t, config)
	require.EqualError(t, err, "common config is required")
}

func TestLoadReturnsUnmarshalError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "stevedore.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{`), 0o600))

	config, err := Load(configPath)

	require.Nil(t, config)
	require.Error(t, err)
	require.Contains(t, err.Error(), "load config")
}

func TestValidateUploadReloadAllowsUploadChanges(t *testing.T) {
	current := testReloadConfig()
	next := testReloadConfig()
	next.Upload = &UploadConfig{DefaultUpstream: "reloaded"}

	require.NoError(t, ValidateUploadReload(current, next))
}

func TestValidateUploadReloadRejectsNonUploadChanges(t *testing.T) {
	tests := map[string]func(*Config){
		"Common":  func(config *Config) { config.Common.WorkingDir = "/changed" },
		"PProf":   func(config *Config) { config.PProf.Open = true },
		"Log":     func(config *Config) { config.Log.Level = "debug" },
		"Server":  func(config *Config) { config.Server.BatchSize++ },
		"Session": func(config *Config) { config.Session.TimeoutSec++ },
	}

	for section, change := range tests {
		t.Run(section, func(t *testing.T) {
			current := testReloadConfig()
			next := testReloadConfig()
			change(next)

			err := ValidateUploadReload(current, next)

			require.EqualError(t, err, "non-Upload config changed: "+section)
		})
	}
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

func TestRouteConfigUsesUpstreamAsOnlyRoutingTarget(t *testing.T) {
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
	require.NoError(t, json.Unmarshal(raw, &config))

	encoded, err := json.Marshal(config)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.NotContains(t, decoded, "DefaultAction")
	require.Equal(t, "drop", decoded["DefaultUpstream"])

	routes, ok := decoded["Routes"].([]any)
	require.True(t, ok)
	require.Len(t, routes, 1)
	route, ok := routes[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "canary", route["Name"])
	require.NotContains(t, route, "Action")
	require.Equal(t, "drop", route["Upstream"])
	require.NotContains(t, route, "KeyBytes")
	require.NotContains(t, route, "Operator")
	require.NotContains(t, route, "Value")
	require.NotContains(t, route, "Mask")

	conditions, ok := route["Conditions"].([]any)
	require.True(t, ok)
	require.Len(t, conditions, 1)
	condition, ok := conditions[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "0x80", condition["Mask"])
}

func TestLogConfigExposesUploadStatsInterval(t *testing.T) {
	raw := []byte(`{
		"Log": {
			"UploadStatsIntervalSec": 60
		}
	}`)
	var config Config
	require.NoError(t, json.Unmarshal(raw, &config))

	encoded, err := json.Marshal(config)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))

	logConfig := decoded["Log"].(map[string]any)
	require.Equal(t, float64(60), logConfig["UploadStatsIntervalSec"])
	require.NotContains(t, logConfig, "RouteStatsIntervalSec")
}

func TestUpstreamConfigNestsFixedKeyUnderExtractor(t *testing.T) {
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
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))

	require.NotContains(t, decoded, "KeyBytes")
	require.NotContains(t, decoded, "KeyMask")
	extractor := decoded["KeyExtractor"].(map[string]any)
	require.Equal(t, "fixed", extractor["Type"])
	fixed := extractor["Fixed"].(map[string]any)
	require.Equal(t, "0:2,4:6", fixed["KeyBytes"])
	require.Equal(t, "0xffffffff", fixed["KeyMask"])
}
