package sd_util

import (
	"github.com/stretchr/testify/assert"
	"testing"
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
