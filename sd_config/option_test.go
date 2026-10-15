package sd_config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionAction(t *testing.T) {
	tests := []struct {
		name   string
		option Option
		action Action
	}{
		{name: "run", option: Option{}, action: ActionRun},
		{name: "test", option: Option{Test: true}, action: ActionTest},
		{name: "version", option: Option{Version: true}, action: ActionVersion},
		{name: "reload", option: Option{Signal: "reload"}, action: ActionSignal},
		{name: "stop", option: Option{Signal: "stop"}, action: ActionSignal},
		{name: "quit", option: Option{Signal: "quit"}, action: ActionSignal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action, err := test.option.Action()

			require.NoError(t, err)
			require.Equal(t, test.action, action)
		})
	}
}

func TestOptionActionRejectsConflicts(t *testing.T) {
	_, err := (&Option{Test: true, Signal: "reload"}).Action()

	require.EqualError(t, err, "version, test, and signal options are mutually exclusive")
}

func TestOptionActionRejectsUnknownSignal(t *testing.T) {
	_, err := (&Option{Signal: "kill"}).Action()

	require.EqualError(t, err, `signal "kill" is invalid`)
}
