package sd_route

import (
	"strconv"
	"strings"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_util"
	"github.com/sirupsen/logrus"
)

type condition struct {
	operator   string
	bytesStart int
	bytesEnd   int
	bytesValue []byte
	mask       []byte
}

func newCondition(routeID int, config sd_config.RouteConditionConfig) condition {
	switch config.Operator {
	case sd_util.BytesOpEqual,
		sd_util.BytesOpNotEqual,
		sd_util.BytesOpOrThenEqual,
		sd_util.BytesOpOrThenNotEqual,
		sd_util.BytesOpAndThenEqual,
		sd_util.BytesOpAndThenNotEqual:
	default:
		logrus.Panicf("operator %s is invalid in route %d", config.Operator, routeID)
	}

	bytesIdx := strings.Split(config.KeyBytes, ":")
	if len(bytesIdx) != 2 {
		logrus.Panicf("key bytes %s is invalid in route %d", config.KeyBytes, routeID)
	}
	bytesStart, err := strconv.Atoi(bytesIdx[0])
	if err != nil {
		logrus.Panicf("bytes start %s is invalid in route %d", bytesIdx[0], routeID)
	}
	bytesEnd, err := strconv.Atoi(bytesIdx[1])
	if err != nil {
		logrus.Panicf("bytes end %s is invalid in route %d", bytesIdx[1], routeID)
	}
	if bytesStart < 0 {
		logrus.Panicf("bytes start %s is negative in route %d", bytesIdx[0], routeID)
	}
	if bytesStart >= bytesEnd {
		logrus.Panicf("bytes start %s is not less than bytes end %s in route %d", bytesIdx[0], bytesIdx[1], routeID)
	}

	bytesValue, err := sd_util.StringToBytes(config.Value)
	if err != nil {
		logrus.Panicf("value %s invalid in route %d: %v", config.Value, routeID, err)
	}
	if len(bytesValue) != bytesEnd-bytesStart {
		logrus.Panicf("value %s length is not equal to bytes length in route %d", config.Value, routeID)
	}

	var mask []byte
	if config.Mask != "" {
		mask, err = sd_util.StringToBytes(config.Mask)
		if err != nil {
			logrus.Panicf("mask %s invalid in route %d: %v", config.Mask, routeID, err)
		}
		if len(mask) != len(bytesValue) {
			logrus.Panicf("mask %s length is not equal to bytes length in route %d", config.Mask, routeID)
		}
		if config.Operator != sd_util.BytesOpEqual && config.Operator != sd_util.BytesOpNotEqual {
			logrus.Panicf("mask is not supported with operator %s in route %d", config.Operator, routeID)
		}
		for i := range bytesValue {
			if bytesValue[i]&^mask[i] != 0 {
				logrus.Panicf("value %s contains bits outside mask %s in route %d", config.Value, config.Mask, routeID)
			}
		}
	}

	return condition{
		operator:   config.Operator,
		bytesStart: bytesStart,
		bytesEnd:   bytesEnd,
		bytesValue: bytesValue,
		mask:       mask,
	}
}

func (c *condition) matchStatus(data []byte) (bool, bool) {
	if len(data) < c.bytesEnd {
		return false, true
	}

	bytes := data[c.bytesStart:c.bytesEnd]
	if c.mask != nil {
		matched := true
		for i := range bytes {
			if bytes[i]&c.mask[i] != c.bytesValue[i] {
				matched = false
				break
			}
		}
		if c.operator == sd_util.BytesOpNotEqual {
			return !matched, false
		}
		return matched, false
	}

	return c.matchBytes(bytes), false
}

func (c *condition) matchBytes(bytes []byte) bool {
	matched := true
	switch c.operator {
	case sd_util.BytesOpEqual, sd_util.BytesOpNotEqual:
		for i := range bytes {
			if bytes[i] != c.bytesValue[i] {
				matched = false
				break
			}
		}
	case sd_util.BytesOpOrThenEqual, sd_util.BytesOpOrThenNotEqual:
		for i := range bytes {
			if bytes[i]|c.bytesValue[i] != c.bytesValue[i] {
				matched = false
				break
			}
		}
	case sd_util.BytesOpAndThenEqual, sd_util.BytesOpAndThenNotEqual:
		for i := range bytes {
			if bytes[i]&c.bytesValue[i] != c.bytesValue[i] {
				matched = false
				break
			}
		}
	}

	switch c.operator {
	case sd_util.BytesOpNotEqual, sd_util.BytesOpOrThenNotEqual, sd_util.BytesOpAndThenNotEqual:
		return !matched
	default:
		return matched
	}
}
