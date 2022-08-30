package sd_util

import "fmt"

// Converts a hex string or bit string into byte slice and a error
func StringToBytes(s string) ([]byte, error) {
	flag := s[:2]
	switch flag {
	case "0x":
		return hexStringToBytes(s[2:])
	case "0b":
		return bitStringToBytes(s[2:])
	}

	return nil, fmt.Errorf("invalid string %s", s)
}

// Converts a hex string into byte slice and a error
func hexStringToBytes(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("invalid length %d of hex string", len(s))
	}

	src := []byte(s)
	i, j := 0, 1
	for ; j < len(src); j += 2 {
		a, err := fromHexChar(src[j-1])
		if err != nil {
			return nil, err
		}
		b, err := fromHexChar(src[j])
		if err != nil {
			return nil, err
		}
		src[i] = (a << 4) | b
		i++
	}

	return src[:i], nil
}

// Converts a hex character into its value and a error
func fromHexChar(c byte) (byte, error) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', nil
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, nil
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, nil
	}

	return 0, fmt.Errorf("invalid byte %#U in hex string", rune(c))
}

// Converts a bit string into byte slice and a error
func bitStringToBytes(s string) ([]byte, error) {
	if len(s)%8 != 0 {
		return nil, fmt.Errorf("invalid length %d of bit string", len(s))
	}

	src := []byte(s)
	i, j := 0, 7
	for ; j < len(src); j += 8 {
		for k := 7; k >= 0; k-- {
			v, err := fromBitChar(src[j-k])
			if err != nil {
				return nil, err
			}
			src[i] = (src[i] << 1) | v
		}
		i++
	}

	return src[:i], nil
}

// Converts a bit character into its value and a error
func fromBitChar(c byte) (byte, error) {
	switch {
	case c == '0':
		return 0, nil
	case c == '1':
		return 1, nil
	}

	return 0, fmt.Errorf("invalid byte %#U in bit string", rune(c))
}
