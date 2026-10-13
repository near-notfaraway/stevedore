package sd_upstream

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/stretchr/testify/require"
)

func TestLengthFieldKeyExtractorExtractsBigEndianKey(t *testing.T) {
	extractor, err := newLengthFieldKeyExtractor(sd_config.LengthFieldKeyConfig{
		KeyStart:     4,
		LengthStart:  1,
		LengthBytes:  2,
		ByteOrder:    "big",
		MaxKeyLength: 8,
	})
	require.NoError(t, err)
	data := []byte{0xaa, 0x00, 0x03, 0xbb, 0x11, 0x22, 0x33, 0xff}

	key, ok := extractor.Extract(data)

	require.True(t, ok)
	require.Equal(t, []byte{0x11, 0x22, 0x33}, key)
	require.Same(t, &data[4], &key[0])
}

func TestLengthFieldKeyExtractorExtractsLittleEndianKey(t *testing.T) {
	extractor, err := newLengthFieldKeyExtractor(sd_config.LengthFieldKeyConfig{
		KeyStart:     3,
		LengthStart:  0,
		LengthBytes:  2,
		ByteOrder:    "little",
		MaxKeyLength: 8,
	})
	require.NoError(t, err)
	data := []byte{0x03, 0x00, 0xaa, 0x11, 0x22, 0x33}

	key, ok := extractor.Extract(data)

	require.True(t, ok)
	require.Equal(t, []byte{0x11, 0x22, 0x33}, key)
}

func TestLengthFieldKeyExtractorReadsFourByteLength(t *testing.T) {
	extractor, err := newLengthFieldKeyExtractor(sd_config.LengthFieldKeyConfig{
		KeyStart:     4,
		LengthStart:  0,
		LengthBytes:  4,
		ByteOrder:    "big",
		MaxKeyLength: 8,
	})
	require.NoError(t, err)
	data := []byte{0x00, 0x00, 0x00, 0x03, 0x11, 0x22, 0x33}

	key, ok := extractor.Extract(data)

	require.True(t, ok)
	require.Equal(t, []byte{0x11, 0x22, 0x33}, key)
}

func TestLengthFieldKeyExtractorRejectsInvalidPayload(t *testing.T) {
	extractor, err := newLengthFieldKeyExtractor(sd_config.LengthFieldKeyConfig{
		KeyStart:     2,
		LengthStart:  0,
		LengthBytes:  1,
		ByteOrder:    "big",
		MaxKeyLength: 4,
	})
	require.NoError(t, err)

	tests := map[string][]byte{
		"missing length": nil,
		"zero length":    {0x00, 0xaa},
		"over maximum":   {0x05, 0xaa, 0x11, 0x22, 0x33, 0x44, 0x55},
		"truncated key":  {0x03, 0xaa, 0x11, 0x22},
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			key, ok := extractor.Extract(data)
			require.False(t, ok)
			require.Nil(t, key)
		})
	}
}

func TestNewLengthFieldKeyExtractorRejectsInvalidConfig(t *testing.T) {
	valid := sd_config.LengthFieldKeyConfig{
		KeyStart:     2,
		LengthStart:  0,
		LengthBytes:  1,
		ByteOrder:    "big",
		MaxKeyLength: 8,
	}
	tests := map[string]sd_config.LengthFieldKeyConfig{
		"negative key start":    withLengthFieldKeyConfig(valid, func(c *sd_config.LengthFieldKeyConfig) { c.KeyStart = -1 }),
		"negative length start": withLengthFieldKeyConfig(valid, func(c *sd_config.LengthFieldKeyConfig) { c.LengthStart = -1 }),
		"invalid length bytes":  withLengthFieldKeyConfig(valid, func(c *sd_config.LengthFieldKeyConfig) { c.LengthBytes = 3 }),
		"invalid byte order":    withLengthFieldKeyConfig(valid, func(c *sd_config.LengthFieldKeyConfig) { c.ByteOrder = "middle" }),
		"missing maximum":       withLengthFieldKeyConfig(valid, func(c *sd_config.LengthFieldKeyConfig) { c.MaxKeyLength = 0 }),
		"oversized maximum":     withLengthFieldKeyConfig(valid, func(c *sd_config.LengthFieldKeyConfig) { c.MaxKeyLength = maxPayloadKeySize + 1 }),
	}

	for name, config := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newLengthFieldKeyExtractor(config)
			require.Error(t, err)
		})
	}
}

func TestLengthFieldKeyExtractorDoesNotAllocate(t *testing.T) {
	extractor, err := newLengthFieldKeyExtractor(sd_config.LengthFieldKeyConfig{
		KeyStart:     2,
		LengthStart:  0,
		LengthBytes:  1,
		ByteOrder:    "big",
		MaxKeyLength: 8,
	})
	require.NoError(t, err)
	data := []byte{0x03, 0xaa, 0x11, 0x22, 0x33}

	allocs := testing.AllocsPerRun(1000, func() {
		_, ok := extractor.Extract(data)
		if !ok {
			t.Fatal("extract failed")
		}
	})

	require.Zero(t, allocs)
}

func withLengthFieldKeyConfig(
	config sd_config.LengthFieldKeyConfig,
	update func(*sd_config.LengthFieldKeyConfig),
) sd_config.LengthFieldKeyConfig {
	update(&config)
	return config
}
