package sd_util

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestStringToBytesConvey(t *testing.T) {
	convey.Convey("Test StringToBytes", t, func() {
		convey.Convey("dispatches by prefix", func() {
			got, err := StringToBytes("0x1234")
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldResemble, []byte{0x12, 0x34})

			got, err = StringToBytes("xx")
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldResemble, []byte("xx"))
		})
	})
}

func TestHexStringToBytesConvey(t *testing.T) {
	convey.Convey("Test hexStringToBytes", t, func() {
		convey.Convey("decodes complete byte pairs", func() {
			got, err := hexStringToBytes("aF")
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldResemble, []byte{0xaf})

			_, err = hexStringToBytes("abc")
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestFromHexCharConvey(t *testing.T) {
	convey.Convey("Test fromHexChar", t, func() {
		convey.Convey("accepts digits and both letter cases", func() {
			for input, expected := range map[byte]byte{'0': 0, '9': 9, 'a': 10, 'F': 15} {
				got, err := fromHexChar(input)
				convey.So(err, convey.ShouldBeNil)
				convey.So(got, convey.ShouldEqual, expected)
			}
			_, err := fromHexChar('g')
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestBitStringToBytesConvey(t *testing.T) {
	convey.Convey("Test bitStringToBytes", t, func() {
		convey.Convey("decodes groups of eight bits", func() {
			got, err := bitStringToBytes("10101010")
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldResemble, []byte{0xaa})

			_, err = bitStringToBytes("1")
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestFromBitCharConvey(t *testing.T) {
	convey.Convey("Test fromBitChar", t, func() {
		convey.Convey("accepts binary digits", func() {
			zero, err := fromBitChar('0')
			convey.So(err, convey.ShouldBeNil)
			convey.So(zero, convey.ShouldEqual, 0)

			one, err := fromBitChar('1')
			convey.So(err, convey.ShouldBeNil)
			convey.So(one, convey.ShouldEqual, 1)

			_, err = fromBitChar('2')
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
