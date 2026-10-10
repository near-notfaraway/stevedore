package sd_upstream

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/stretchr/testify/require"
)

func TestRouteMatch(t *testing.T) {
	route := NewRoute(3, sd_config.RouteConfig{
		KeyBytes: "1:3",
		Operator: "==",
		Value:    "0x1234",
		Upstream: "target",
	})

	require.Equal(t, 3, route.id)
	require.Equal(t, "target", route.upstream)
	require.True(t, route.Match([]byte{0x00, 0x12, 0x34}))
	require.False(t, route.Match([]byte{0x00, 0x12, 0x35}))
	require.False(t, route.Match([]byte{0x00, 0x12}))
}

func TestRouteMatchReturnsFalseForInvalidOperator(t *testing.T) {
	route := NewRoute(0, sd_config.RouteConfig{
		KeyBytes: "0:1",
		Operator: "invalid",
		Value:    "0x01",
		Upstream: "target",
	})

	require.False(t, route.Match([]byte{0x01}))
}

func TestNewRouteRejectsInvalidConfig(t *testing.T) {
	tests := []sd_config.RouteConfig{
		{KeyBytes: "start:1", Operator: "==", Value: "0x01"},
		{KeyBytes: "0:end", Operator: "==", Value: "0x01"},
		{KeyBytes: "1:1", Operator: "==", Value: ""},
		{KeyBytes: "0:1", Operator: "==", Value: "invalid"},
		{KeyBytes: "0:2", Operator: "==", Value: "0x01"},
	}

	for _, config := range tests {
		require.Panics(t, func() {
			NewRoute(0, config)
		})
	}
}

func TestNewRouteRejectsNegativeByteStart(t *testing.T) {
	require.Panics(t, func() {
		NewRoute(0, sd_config.RouteConfig{
			KeyBytes: "-1:1",
			Operator: "==",
			Value:    "0x0001",
			Upstream: "target",
		})
	})
}
