package sd_util

import "fmt"

const (
	BytesOpEqual           = "=="
	BytesOpNotEqual        = "!=="
	BytesOpOrThenEqual     = "|="
	BytesOpOrThenNotEqual  = "!|="
	BytesOpAndThenEqual    = "&="
	BytesOpAndThenNotEqual = "!&="
)

// Return bytes operate result and a error
func BytesOperate(symbol string, leftVal, rightVal []byte) (bool, error) {
	if len(leftVal) != len(rightVal) {
		return false, fmt.Errorf("should be the same length for operation")
	}

	var rst bool
	switch symbol {
	case BytesOpEqual:
		rst = bytesEqual(leftVal, rightVal)
	case BytesOpNotEqual:
		rst = !bytesEqual(leftVal, rightVal)
	case BytesOpOrThenEqual:
		rst = bytesOrThenEqual(leftVal, rightVal)
	case BytesOpOrThenNotEqual:
		rst = !bytesOrThenEqual(leftVal, rightVal)
	case BytesOpAndThenEqual:
		rst = bytesAndThenEqual(leftVal, rightVal)
	case BytesOpAndThenNotEqual:
		rst = !bytesAndThenEqual(leftVal, rightVal)
	default:
		return false, fmt.Errorf("invalid operator %s", symbol)
	}

	return rst, nil
}

// Return l == r
func bytesEqual(leftVal, rightVal []byte) bool {
	return string(leftVal) == string(rightVal)
}

// Return (l | r) == r
// If a bit of r is 0, then related bit of l must be 0
func bytesOrThenEqual(leftVal, rightVal []byte) bool {
	tmp := make([]byte, len(leftVal))
	for i := 0; i < len(tmp); i++ {
		tmp[i] = leftVal[i] | rightVal[i]
	}

	return string(tmp) == string(rightVal)
}

// Return (l & r) == r
// If a bit of r is 1, then related bit of l must be 1
func bytesAndThenEqual(leftVal, rightVal []byte) bool {
	tmp := make([]byte, len(leftVal))
	for i := 0; i < len(tmp); i++ {
		tmp[i] = leftVal[i] & rightVal[i]
	}

	return string(tmp) == string(rightVal)
}
