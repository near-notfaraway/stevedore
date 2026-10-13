package sd_config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

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
