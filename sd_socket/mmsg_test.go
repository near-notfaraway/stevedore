package sd_socket

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestNormalizeMMsgSizeConvey(t *testing.T) {
	convey.Convey("Test normalizeMMsgSize", t, func() {
		convey.Convey("enforces minimums", func() {
			batch, mtu := normalizeMMsgSize(1, 1)
			convey.So(batch, convey.ShouldEqual, minMMsgContainerBatchNum)
			convey.So(mtu, convey.ShouldEqual, minMMsgContainerMTU)
		})
	})
}
