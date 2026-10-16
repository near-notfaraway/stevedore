package sd_config

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestOptionActionConvey(t *testing.T) {
	convey.Convey("Test Option.Action", t, func() {
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
			convey.Convey("returns "+test.name+" action", func() {
				action, err := test.option.Action()

				convey.So(err, convey.ShouldBeNil)
				convey.So(action, convey.ShouldEqual, test.action)
			})
		}

		convey.Convey("rejects conflicting options", func() {
			_, err := (&Option{Test: true, Signal: "reload"}).Action()

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual,
				"version, test, and signal options are mutually exclusive")
		})

		convey.Convey("rejects unknown signals", func() {
			_, err := (&Option{Signal: "kill"}).Action()

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldEqual, `signal "kill" is invalid`)
		})
	})
}
