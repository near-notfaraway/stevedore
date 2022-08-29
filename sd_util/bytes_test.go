package sd_util

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestStringToBytes1(t *testing.T) {
	s := "0x1234"
	bytes, err := StringToBytes(s)
	assert.Nil(t, err)
	fmt.Printf("%v", bytes)
}

func TestStringToBytes2(t *testing.T) {
	s := "0b1011101110111001"
	bytes, err := StringToBytes(s)
	assert.Nil(t, err)
	fmt.Printf("%v", bytes)
}

