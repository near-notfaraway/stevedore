package sd_upstream

import (
	"fmt"

	"github.com/near-notfaraway/stevedore/sd_config"
)

const (
	byteOrderBig    = "big"
	byteOrderLittle = "little"
)

type lengthFieldKeyExtractor struct {
	keyStart     int
	lengthStart  int
	lengthEnd    int
	littleEndian bool
	maxKeyLength int
}

func newLengthFieldKeyExtractor(config sd_config.LengthFieldKeyConfig) (*lengthFieldKeyExtractor, error) {
	if config.KeyStart < 0 {
		return nil, fmt.Errorf("key start %d is negative", config.KeyStart)
	}
	if config.LengthStart < 0 {
		return nil, fmt.Errorf("length start %d is negative", config.LengthStart)
	}
	switch config.LengthBytes {
	case 1, 2, 4:
	default:
		return nil, fmt.Errorf("length bytes %d is not one of 1, 2, or 4", config.LengthBytes)
	}
	if config.LengthStart > int(^uint(0)>>1)-config.LengthBytes {
		return nil, fmt.Errorf("length field end overflows int")
	}

	littleEndian := false
	switch config.ByteOrder {
	case byteOrderBig:
	case byteOrderLittle:
		littleEndian = true
	default:
		return nil, fmt.Errorf("byte order %q is invalid", config.ByteOrder)
	}
	if config.MaxKeyLength < 1 || config.MaxKeyLength > maxPayloadKeySize {
		return nil, fmt.Errorf("max key length %d is outside [1, %d]", config.MaxKeyLength, maxPayloadKeySize)
	}

	return &lengthFieldKeyExtractor{
		keyStart:     config.KeyStart,
		lengthStart:  config.LengthStart,
		lengthEnd:    config.LengthStart + config.LengthBytes,
		littleEndian: littleEndian,
		maxKeyLength: config.MaxKeyLength,
	}, nil
}

func (e *lengthFieldKeyExtractor) extract(data []byte) ([]byte, bool) {
	if len(data) < e.lengthEnd {
		return nil, false
	}

	var keyLengthValue uint64
	lengthBytes := data[e.lengthStart:e.lengthEnd]
	if e.littleEndian {
		for i := len(lengthBytes) - 1; i >= 0; i-- {
			keyLengthValue = keyLengthValue<<8 | uint64(lengthBytes[i])
		}
	} else {
		for _, value := range lengthBytes {
			keyLengthValue = keyLengthValue<<8 | uint64(value)
		}
	}
	if keyLengthValue < 1 || keyLengthValue > uint64(e.maxKeyLength) {
		return nil, false
	}
	keyLength := int(keyLengthValue)
	if keyLength > len(data) || e.keyStart > len(data)-keyLength {
		return nil, false
	}
	return data[e.keyStart : e.keyStart+keyLength], true
}
