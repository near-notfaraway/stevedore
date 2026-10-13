package main

import (
	"errors"
	"flag"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_server"
	"github.com/near-notfaraway/stevedore/sd_util"
	"github.com/smartystreets/goconvey/convey"
)

func TestMainConvey(t *testing.T) {
	convey.Convey("Test main", t, func() {
		convey.Convey("loads configuration and starts the server", func() {
			configPath := "mock.json"
			server := &sd_server.Server{}
			unmarshalCalled := false
			loggerCalled := false

			patches := gomonkey.NewPatches()
			patches.ApplyFuncReturn(flag.String, &configPath)
			patches.ApplyFunc(flag.Parse, func() {})
			patches.ApplyFunc(
				sd_util.UnmarshalFile,
				func(filename string, value interface{}) error {
					unmarshalCalled = true
					config := value.(*sd_config.Config)
					config.PProf = &sd_config.PProfConfig{}
					config.Log = &sd_config.LogConfig{}
					return nil
				},
			)
			patches.ApplyFunc(
				sd_util.InitLogger,
				func(*sd_config.LogConfig) error {
					loggerCalled = true
					return nil
				},
			)
			patches.ApplyFuncReturn(sd_server.NewServer, server)
			patches.ApplyMethodReturn(server, "ListenAndServe", errors.New("mock server stopped"))
			defer patches.Reset()

			convey.So(main, convey.ShouldPanic)
			convey.So(unmarshalCalled, convey.ShouldBeTrue)
			convey.So(loggerCalled, convey.ShouldBeTrue)
		})
	})
}
