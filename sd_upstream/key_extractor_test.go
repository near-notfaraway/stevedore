package sd_upstream

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/smartystreets/goconvey/convey"
)

func TestNewKeyExtractorConvey(t *testing.T) {
	convey.Convey("Test newKeyExtractor", t, func() {
		convey.Convey("dispatches fixed extraction", func() {
			extractor, err := newKeyExtractor(&sd_config.UpstreamConfig{
				KeyExtractor: &sd_config.KeyExtractorConfig{
					Type:  keyExtractorTypeFixed,
					Fixed: &sd_config.FixedKeyConfig{KeyBytes: "0:1"},
				},
			})
			convey.So(err, convey.ShouldBeNil)
			convey.So(extractor, convey.ShouldHaveSameTypeAs, &fixedKeyExtractor{})
		})
	})
}
