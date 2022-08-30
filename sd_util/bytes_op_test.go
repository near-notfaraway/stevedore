package sd_util

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

// invalid value or operator
func TestBytesOperate1(t *testing.T) {
	leftVal := []byte{113, 24, 255, 21}
	rightVal := []byte{113, 24, 255}
	_, err := BytesOperate("==", leftVal, rightVal)
	assert.NotNil(t, err)

	leftVal = []byte{113, 24, 255}
	_, err = BytesOperate("!=", leftVal, rightVal)
	assert.NotNil(t, err)
}

// equal operation
func TestBytesOperate2(t *testing.T) {
	leftVal := []byte{113, 24, 255}
	rightVal := []byte{113, 24, 255}
	rst, err := BytesOperate("==", leftVal, rightVal)
	assert.Nil(t, err)
	assert.Equal(t, true, rst)

	rst, err = BytesOperate("!==", leftVal, rightVal)
	assert.Nil(t, err)
	assert.Equal(t, false, rst)
}

// or then equal operation: bits of right value is 0, make sure related bits of right value are 0
func TestBytesOperate3(t *testing.T) {
	leftVal := []byte{0, 0x44}
	rightVal := []byte{0xff, 0x44}
	rst, err := BytesOperate("|=", leftVal, rightVal)
	assert.Nil(t, err)
	assert.Equal(t, true, rst)

	rst, err = BytesOperate("!|=", leftVal, rightVal)
	assert.Nil(t, err)
	assert.Equal(t, false, rst)
}

// and then equal operation: bits of right value is 1, make sure related bits of right value are 1
func TestBytesOperate4(t *testing.T) {
	leftVal := []byte{0x22, 0xfc}
	rightVal := []byte{0x00, 0x0c}
	rst, err := BytesOperate("&=", leftVal, rightVal)
	assert.Nil(t, err)
	assert.Equal(t, true, rst)

	rst, err = BytesOperate("!&=", leftVal, rightVal)
	assert.Nil(t, err)
	assert.Equal(t, false, rst)
}
