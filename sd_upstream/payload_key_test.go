package sd_upstream

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPayloadKeyExtractCombinesSegments(t *testing.T) {
	key, err := newPayloadKey("0:2,4:6", "")
	require.NoError(t, err)

	got, ok := key.Extract([]byte{0x12, 0x34, 0xaa, 0xbb, 0x56, 0x78})

	require.True(t, ok)
	require.Equal(t, []byte{0x12, 0x34, 0x56, 0x78}, got)
}

func TestPayloadKeyExtractAppliesHexMask(t *testing.T) {
	key, err := newPayloadKey("0:2,4:6", "0xff0f00f0")
	require.NoError(t, err)

	got, ok := key.Extract([]byte{0x12, 0x34, 0xaa, 0xbb, 0x56, 0x78})

	require.True(t, ok)
	require.Equal(t, []byte{0x12, 0x04, 0x00, 0x70}, got)
}

func TestPayloadKeyExtractAppliesBitMask(t *testing.T) {
	key, err := newPayloadKey("0:1", "0b11110000")
	require.NoError(t, err)

	got, ok := key.Extract([]byte{0xab})

	require.True(t, ok)
	require.Equal(t, []byte{0xa0}, got)
}

func TestPayloadKeyExtractRejectsShortPayload(t *testing.T) {
	key, err := newPayloadKey("0:2,4:6", "")
	require.NoError(t, err)

	got, ok := key.Extract([]byte{0x12, 0x34, 0xaa, 0xbb, 0x56})

	require.False(t, ok)
	require.Nil(t, got)
}

func TestPayloadKeyExtractSingleSegmentWithoutCopy(t *testing.T) {
	key, err := newPayloadKey("1:3", "")
	require.NoError(t, err)
	data := []byte{0x00, 0x12, 0x34, 0x00}

	got, ok := key.Extract(data)

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
			_, err := newPayloadKey(keyBytes, "")
			require.Error(t, err)
		})
	}
}

func TestNewPayloadKeyRejectsOversizedCombinedKey(t *testing.T) {
	_, err := newPayloadKey("0:65536,0:1", "")

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
			_, err := newPayloadKey("0:1", mask)
			require.Error(t, err)
		})
	}
}
