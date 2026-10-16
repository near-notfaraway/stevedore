package sd_route

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_util"
	"github.com/smartystreets/goconvey/convey"
)

func TestNewConditionConvey(t *testing.T) {
	convey.Convey("Test newCondition", t, func() {
		config := sd_config.RouteConfig{
			Name: "matched",
			Conditions: []sd_config.RouteConditionConfig{{
				KeyBytes: "0:1",
				Operator: sd_util.BytesOpEqual,
				Value:    "0x01",
			}},
			Upstream: "target",
		}
		convey.Convey("parses a valid condition", func() {
			got, err := newCondition(0, config.Conditions[0])
			convey.So(err, convey.ShouldBeNil)
			convey.So(got.bytesStart, convey.ShouldEqual, 0)
			convey.So(got.bytesEnd, convey.ShouldEqual, 1)
			convey.So(got.bytesValue, convey.ShouldResemble, []byte{1})
		})
	})
}

func TestConditionMatchStatusConvey(t *testing.T) {
	convey.Convey("Test condition.matchStatus", t, func() {
		config := sd_config.RouteConfig{
			Name: "matched",
			Conditions: []sd_config.RouteConditionConfig{{
				KeyBytes: "0:1",
				Operator: sd_util.BytesOpEqual,
				Value:    "0x01",
			}},
			Upstream: "target",
		}
		convey.Convey("reports short and matching payloads", func() {
			c, err := newCondition(0, config.Conditions[0])
			convey.So(err, convey.ShouldBeNil)
			matched, short := c.matchStatus(nil)
			convey.So(matched, convey.ShouldBeFalse)
			convey.So(short, convey.ShouldBeTrue)

			matched, short = c.matchStatus([]byte{1})
			convey.So(matched, convey.ShouldBeTrue)
			convey.So(short, convey.ShouldBeFalse)
		})
	})
}

func TestConditionMatchBytesConvey(t *testing.T) {
	convey.Convey("Test condition.matchBytes", t, func() {
		convey.Convey("applies the configured operator", func() {
			c := condition{operator: sd_util.BytesOpAndThenEqual, bytesValue: []byte{1}}
			convey.So(c.matchBytes([]byte{3}), convey.ShouldBeTrue)
			convey.So(c.matchBytes([]byte{2}), convey.ShouldBeFalse)
		})
	})
}
