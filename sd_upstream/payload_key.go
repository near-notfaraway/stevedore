package sd_upstream

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/near-notfaraway/stevedore/sd_util"
)

const maxPayloadKeySize = 1 << 16

type payloadKeySegment struct {
	start int
	end   int
}

type payloadKey struct {
	segments           []payloadKeySegment
	mask               []byte
	buf                []byte
	requiredPayloadLen int
}

func newPayloadKey(keyBytes, keyMask string) (*payloadKey, error) {
	rawSegments := strings.Split(keyBytes, ",")
	segments := make([]payloadKeySegment, 0, len(rawSegments))
	keyLen := 0
	requiredPayloadLen := 0

	for _, rawSegment := range rawSegments {
		segment, err := parsePayloadKeySegment(strings.TrimSpace(rawSegment))
		if err != nil {
			return nil, err
		}
		segmentLen := segment.end - segment.start
		if segmentLen > maxPayloadKeySize-keyLen {
			return nil, fmt.Errorf("payload key length exceeds %d", maxPayloadKeySize)
		}
		keyLen += segmentLen
		if segment.end > requiredPayloadLen {
			requiredPayloadLen = segment.end
		}
		segments = append(segments, segment)
	}

	mask, err := parsePayloadKeyMask(strings.TrimSpace(keyMask), keyLen)
	if err != nil {
		return nil, err
	}

	var buf []byte
	if len(segments) > 1 || mask != nil {
		buf = make([]byte, keyLen)
	}
	return &payloadKey{
		segments:           segments,
		mask:               mask,
		buf:                buf,
		requiredPayloadLen: requiredPayloadLen,
	}, nil
}

func parsePayloadKeySegment(value string) (payloadKeySegment, error) {
	bounds := strings.Split(value, ":")
	if len(bounds) != 2 {
		return payloadKeySegment{}, fmt.Errorf("key segment %q is invalid", value)
	}

	start, err := strconv.Atoi(bounds[0])
	if err != nil {
		return payloadKeySegment{}, fmt.Errorf("key start %q is invalid", bounds[0])
	}
	end, err := strconv.Atoi(bounds[1])
	if err != nil {
		return payloadKeySegment{}, fmt.Errorf("key end %q is invalid", bounds[1])
	}
	if start < 0 {
		return payloadKeySegment{}, fmt.Errorf("key start %d is negative", start)
	}
	if start >= end {
		return payloadKeySegment{}, fmt.Errorf("key start %d >= key end %d", start, end)
	}
	return payloadKeySegment{start: start, end: end}, nil
}

func parsePayloadKeyMask(value string, keyLen int) ([]byte, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) < 2 {
		return nil, fmt.Errorf("key mask %q is invalid", value)
	}
	mask, err := sd_util.StringToBytes(value)
	if err != nil {
		return nil, fmt.Errorf("key mask %q is invalid: %w", value, err)
	}
	if len(mask) != keyLen {
		return nil, fmt.Errorf("key mask length %d does not match key length %d", len(mask), keyLen)
	}
	return mask, nil
}

func (k *payloadKey) Extract(data []byte) ([]byte, bool) {
	if len(data) < k.requiredPayloadLen {
		return nil, false
	}
	if len(k.segments) == 1 && k.mask == nil {
		segment := k.segments[0]
		return data[segment.start:segment.end], true
	}

	offset := 0
	for _, segment := range k.segments {
		offset += copy(k.buf[offset:], data[segment.start:segment.end])
	}
	for i, mask := range k.mask {
		k.buf[i] &= mask
	}
	return k.buf, true
}
