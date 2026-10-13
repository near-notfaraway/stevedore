package sd_util

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

// Hex string should be process succeed
func TestStringToBytes1(t *testing.T) {
	s := "0x1234"
	bytes, err := StringToBytes(s)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(bytes))
	assert.Equal(t, byte(18), bytes[0])
	assert.Equal(t, byte(52), bytes[1])

	s = "0x12345"
	_, err = StringToBytes(s)
	assert.NotNil(t, err)

	s = "0x123g"
	_, err = StringToBytes(s)
	assert.NotNil(t, err)

	s = "1234"
	_, err = StringToBytes(s)
	assert.NotNil(t, err)
}

// Bit string should be process succeed
func TestStringToBytes2(t *testing.T) {
	s := "0b1011101110111001"
	bytes, err := StringToBytes(s)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(bytes))
	assert.Equal(t, byte(187), bytes[0])
	assert.Equal(t, byte(185), bytes[1])

	s = "0b10111011101110011"
	_, err = StringToBytes(s)
	assert.NotNil(t, err)

	s = "0b101110111011102"
	_, err = StringToBytes(s)
	assert.NotNil(t, err)
}

func TestStringToBytesConvey(t *testing.T) {
	convey.Convey("Test StringToBytes", t, func() {
		convey.Convey("dispatches by prefix", func() {
			got, err := StringToBytes("0x1234")
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldResemble, []byte{0x12, 0x34})

			_, err = StringToBytes("xx")
			convey.So(err, convey.ShouldNotBeNil)
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
