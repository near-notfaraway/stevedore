package sd_route

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/stretchr/testify/require"
)

func TestRouterMatchesMaskedValue(t *testing.T) {
	router := NewRouter("default", []sd_config.RouteConfig{
		{
			Conditions: []sd_config.RouteConditionConfig{
				{KeyBytes: "0:1", Operator: "==", Value: "0x80", Mask: "0xc0"},
			},
			Upstream: "target",
		},
	}, false)

	require.Equal(t, "target", router.Route([]byte{0x8f}))
	require.Equal(t, "default", router.Route([]byte{0xcf}))
}

func TestRouterRouteDoesNotAllocate(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		value    string
		mask     string
		data     byte
	}{
		{name: "equal", operator: "==", value: "0x03", data: 0x03},
		{name: "not equal", operator: "!==", value: "0x02", data: 0x03},
		{name: "or equal", operator: "|=", value: "0x03", data: 0x03},
		{name: "or not equal", operator: "!|=", value: "0x03", data: 0x04},
		{name: "and equal", operator: "&=", value: "0x01", data: 0x03},
		{name: "and not equal", operator: "!&=", value: "0x01", data: 0x02},
		{name: "masked equal", operator: "==", value: "0x80", mask: "0xc0", data: 0x8f},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := NewRouter("default", []sd_config.RouteConfig{
				{
					Conditions: []sd_config.RouteConditionConfig{
						{
							KeyBytes: "0:1",
							Operator: test.operator,
							Value:    test.value,
							Mask:     test.mask,
						},
					},
					Upstream: "target",
				},
			}, false)
			data := []byte{test.data}
			var upstream string

			allocs := testing.AllocsPerRun(1000, func() {
				upstream = router.Route(data)
			})

			require.Zero(t, allocs)
			require.Equal(t, "target", upstream)
		})
	}
}

func TestNewRouterRejectsInvalidOperator(t *testing.T) {
	require.Panics(t, func() {
		NewRouter("default", []sd_config.RouteConfig{
			{
				Conditions: []sd_config.RouteConditionConfig{
					{KeyBytes: "0:1", Operator: "invalid", Value: "0x01"},
				},
				Upstream: "target",
			},
		}, false)
	})
}

func TestNewRouterRejectsMalformedKeyBytes(t *testing.T) {
	for _, keyBytes := range []string{"0", "0:1:2"} {
		require.Panics(t, func() {
			NewRouter("default", []sd_config.RouteConfig{
				{
					Conditions: []sd_config.RouteConditionConfig{
						{KeyBytes: keyBytes, Operator: "==", Value: "0x01"},
					},
					Upstream: "target",
				},
			}, false)
		})
	}
}

func TestNewRouterRejectsInvalidMaskSemantics(t *testing.T) {
	tests := []sd_config.RouteConfig{
		{
			Conditions: []sd_config.RouteConditionConfig{
				{KeyBytes: "0:1", Operator: "&=", Value: "0x80", Mask: "0xc0"},
			},
			Upstream: "target",
		},
		{
			Conditions: []sd_config.RouteConditionConfig{
				{KeyBytes: "0:1", Operator: "==", Value: "0xc0", Mask: "0x80"},
			},
			Upstream: "target",
		},
	}

	for _, config := range tests {
		require.Panics(t, func() {
			NewRouter("default", []sd_config.RouteConfig{config}, false)
		})
	}
}

func TestNewRouterRejectsInvalidConditionConfig(t *testing.T) {
	tests := []sd_config.RouteConfig{
		{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "start:1", Operator: "==", Value: "0x01"}}},
		{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:end", Operator: "==", Value: "0x01"}}},
		{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "1:1", Operator: "==", Value: ""}}},
		{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:1", Operator: "==", Value: "invalid"}}},
		{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "0:2", Operator: "==", Value: "0x01"}}},
		{Conditions: []sd_config.RouteConditionConfig{{KeyBytes: "-1:1", Operator: "==", Value: "0x0001"}}},
	}

	for _, config := range tests {
		require.Panics(t, func() {
			NewRouter("", []sd_config.RouteConfig{config}, false)
		})
	}
}
