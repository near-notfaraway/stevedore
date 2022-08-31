package sd_upstream

import (
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_util"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
)

//------------------------------------------------------------------------------
// Route: Used to choose upstream according to data
//------------------------------------------------------------------------------

type Route struct {
	id         int    // unique id
	operator   string // bytes operation type
	bytesStart int    // start index used to extract data
	bytesEnd   int    // end index used to extract data
	bytesValue []byte // bytes used to operate with data
	upstream   string // target upstream
}

func NewRoute(id int, config sd_config.RouteConfig) *Route {
	// init bytes start and bytes end
	bytesIdx := strings.Split(config.KeyBytes, ":")
	bytesStart, err := strconv.Atoi(bytesIdx[0])
	if err != nil {
		logrus.Panicf("bytes start %s is invalid in route %d", bytesIdx[0], id)
	}
	bytesEnd, err := strconv.Atoi(bytesIdx[1])
	if err != nil {
		logrus.Panicf("bytes end %s is invalid in route %d", bytesIdx[1], id)
	}
	if bytesStart >= bytesEnd {
		logrus.Panicf("bytes start %s is not less than bytes end %s in route %d", bytesIdx[0], bytesIdx[1], id)
	}

	// init bytes value
	bytesValue, err := sd_util.StringToBytes(config.Value)
	if err != nil {
		logrus.Panicf("value %s invalid in route %d: %w", config.Value, id, err)
	}
	if len(bytesValue) != (bytesEnd - bytesStart) {
		logrus.Panicf("value length is not bytes length in route %d: %w", config.Value, id, err)
	}

	return &Route{
		id:         id,
		operator:   config.Operator,
		bytesStart: bytesStart,
		bytesEnd:   bytesEnd,
		bytesValue: bytesValue,
		upstream:   config.Upstream,
	}
}

func (r *Route) Match(data []byte) bool {
	// data too short
	if len(data) < r.bytesEnd {
		return false
	}

	// extract bytes and operate
	bytes := data[r.bytesStart:r.bytesEnd]
	matched, err := sd_util.BytesOperate(r.operator, bytes, r.bytesValue)
	if err != nil {
		logrus.Errorf("route match operation failed: %w", err)
	}

	return matched
}
