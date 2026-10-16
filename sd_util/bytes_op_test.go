package sd_util

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestBytesOperateConvey(t *testing.T) {
	convey.Convey("Test bytesOperate", t, func() {
		convey.Convey("rejects invalid values and operators", func() {
			_, err := bytesOperate("==", []byte{113, 24, 255, 21}, []byte{113, 24, 255})
			convey.So(err, convey.ShouldNotBeNil)

			_, err = bytesOperate("!=", []byte{113, 24, 255}, []byte{113, 24, 255})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("compares equal values", func() {
			leftVal := []byte{113, 24, 255}
			rightVal := []byte{113, 24, 255}
			result, err := bytesOperate("==", leftVal, rightVal)
			convey.So(err, convey.ShouldBeNil)
			convey.So(result, convey.ShouldBeTrue)

			result, err = bytesOperate("!==", leftVal, rightVal)
			convey.So(err, convey.ShouldBeNil)
			convey.So(result, convey.ShouldBeFalse)
		})

		convey.Convey("compares OR-masked values", func() {
			leftVal := []byte{0, 0x44}
			rightVal := []byte{0xff, 0x44}
			result, err := bytesOperate("|=", leftVal, rightVal)
			convey.So(err, convey.ShouldBeNil)
			convey.So(result, convey.ShouldBeTrue)

			result, err = bytesOperate("!|=", leftVal, rightVal)
			convey.So(err, convey.ShouldBeNil)
			convey.So(result, convey.ShouldBeFalse)
		})

		convey.Convey("compares AND-masked values", func() {
			leftVal := []byte{0x22, 0xfc}
			rightVal := []byte{0x00, 0x0c}
			result, err := bytesOperate("&=", leftVal, rightVal)
			convey.So(err, convey.ShouldBeNil)
			convey.So(result, convey.ShouldBeTrue)

			result, err = bytesOperate("!&=", leftVal, rightVal)
			convey.So(err, convey.ShouldBeNil)
			convey.So(result, convey.ShouldBeFalse)
		})

		convey.Convey("dispatches supported operators", func() {
			got, err := bytesOperate(BytesOpEqual, []byte{0x01}, []byte{0x01})
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldBeTrue)

			_, err = bytesOperate("invalid", []byte{0x01}, []byte{0x01})
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestBytesEqualConvey(t *testing.T) {
	convey.Convey("Test bytesEqual", t, func() {
		convey.Convey("compares byte content", func() {
			convey.So(bytesEqual([]byte{1, 2}, []byte{1, 2}), convey.ShouldBeTrue)
			convey.So(bytesEqual([]byte{1, 2}, []byte{1, 3}), convey.ShouldBeFalse)
		})
	})
}

func TestBytesOrThenEqualConvey(t *testing.T) {
	convey.Convey("Test bytesOrThenEqual", t, func() {
		convey.Convey("checks subset bits", func() {
			convey.So(bytesOrThenEqual([]byte{0x01}, []byte{0x03}), convey.ShouldBeTrue)
			convey.So(bytesOrThenEqual([]byte{0x04}, []byte{0x03}), convey.ShouldBeFalse)
		})
	})
}

func TestBytesAndThenEqualConvey(t *testing.T) {
	convey.Convey("Test bytesAndThenEqual", t, func() {
		convey.Convey("checks required bits", func() {
			convey.So(bytesAndThenEqual([]byte{0x03}, []byte{0x01}), convey.ShouldBeTrue)
			convey.So(bytesAndThenEqual([]byte{0x02}, []byte{0x01}), convey.ShouldBeFalse)
		})
	})
}
