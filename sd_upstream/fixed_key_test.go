package sd_upstream

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestPayloadKeyExtractCombinesSegments(t *testing.T) {
	key, err := newFixedKeyExtractor("0:2,4:6", "")
	require.NoError(t, err)

	got, ok := key.extract([]byte{0x12, 0x34, 0xaa, 0xbb, 0x56, 0x78})

	require.True(t, ok)
	require.Equal(t, []byte{0x12, 0x34, 0x56, 0x78}, got)
}

func TestPayloadKeyExtractAppliesHexMask(t *testing.T) {
	key, err := newFixedKeyExtractor("0:2,4:6", "0xff0f00f0")
	require.NoError(t, err)

	got, ok := key.extract([]byte{0x12, 0x34, 0xaa, 0xbb, 0x56, 0x78})

	require.True(t, ok)
	require.Equal(t, []byte{0x12, 0x04, 0x00, 0x70}, got)
}

func TestPayloadKeyExtractAppliesBitMask(t *testing.T) {
	key, err := newFixedKeyExtractor("0:1", "0b11110000")
	require.NoError(t, err)

	got, ok := key.extract([]byte{0xab})

	require.True(t, ok)
	require.Equal(t, []byte{0xa0}, got)
}

func TestPayloadKeyExtractRejectsShortPayload(t *testing.T) {
	key, err := newFixedKeyExtractor("0:2,4:6", "")
	require.NoError(t, err)

	got, ok := key.extract([]byte{0x12, 0x34, 0xaa, 0xbb, 0x56})

	require.False(t, ok)
	require.Nil(t, got)
}

func TestPayloadKeyExtractSingleSegmentWithoutCopy(t *testing.T) {
	key, err := newFixedKeyExtractor("1:3", "")
	require.NoError(t, err)
	data := []byte{0x00, 0x12, 0x34, 0x00}

	got, ok := key.extract(data)

	require.True(t, ok)
	require.Equal(t, []byte{0x12, 0x34}, got)
	require.Same(t, &data[1], &got[0])
}

func TestNewPayloadKeyRejectsInvalidSegments(t *testing.T) {
	tests := []string{
		"",
		"0",
		"0:1:2",
		"start:1",
		"0:end",
		"-1:1",
		"1:1",
		"2:1",
		"0:1,",
	}

	for _, keyBytes := range tests {
		t.Run(keyBytes, func(t *testing.T) {
			_, err := newFixedKeyExtractor(keyBytes, "")
			require.Error(t, err)
		})
	}
}

func TestNewPayloadKeyRejectsOversizedCombinedKey(t *testing.T) {
	_, err := newFixedKeyExtractor("0:65536,0:1", "")

	require.Error(t, err)
}

func TestNewPayloadKeyRejectsInvalidMask(t *testing.T) {
	tests := []string{
		"x",
		"mask",
		"0x0",
		"0xffff",
		"0b0000000x",
	}

	for _, mask := range tests {
		t.Run(mask, func(t *testing.T) {
			_, err := newFixedKeyExtractor("0:1", mask)
			require.Error(t, err)
		})
	}
}

func TestNewFixedKeyExtractorConvey(t *testing.T) {
	convey.Convey("Test newFixedKeyExtractor", t, func() {
		convey.Convey("parses segments and masks", func() {
			extractor, err := newFixedKeyExtractor("0:1,2:3", "0xffff")
			convey.So(err, convey.ShouldBeNil)
			convey.So(extractor.segments, convey.ShouldHaveLength, 2)
			convey.So(extractor.mask, convey.ShouldResemble, []byte{0xff, 0xff})
		})
	})
}

func TestFixedKeyExtractorExtractConvey(t *testing.T) {
	convey.Convey("Test fixedKeyExtractor.extract", t, func() {
		convey.Convey("combines configured segments", func() {
			extractor, _ := newFixedKeyExtractor("0:1,2:3", "")
			got, ok := extractor.extract([]byte{1, 9, 2})
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(got, convey.ShouldResemble, []byte{1, 2})
		})
	})
}

func TestParsePayloadKeySegmentConvey(t *testing.T) {
	convey.Convey("Test parsePayloadKeySegment", t, func() {
		convey.Convey("validates and parses bounds", func() {
			got, err := parsePayloadKeySegment("1:3")
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldResemble, fixedKeySegment{start: 1, end: 3})
			_, err = parsePayloadKeySegment("3:1")
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestParsePayloadKeyMaskConvey(t *testing.T) {
	convey.Convey("Test parsePayloadKeyMask", t, func() {
		convey.Convey("validates decoded length", func() {
			got, err := parsePayloadKeyMask("0xff", 1)
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldResemble, []byte{0xff})
			_, err = parsePayloadKeyMask("0xffff", 1)
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
