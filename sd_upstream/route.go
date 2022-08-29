package sd_upstream

import (
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
)

type Route struct {
	id       int
	operator string
	keyStart int
	keyEnd   int
	value    string
	upstream string
}

func NewRoute(id int, config sd_config.RouteConfig) *Route {
	// init key start and key end
	keyIdx := strings.Split(config.KeyBytes, ":")
	keyStart, err := strconv.Atoi(keyIdx[0])
	if err != nil {
		logrus.Panicf("key start %s is invalid in route %d", keyIdx[0], id)
	}
	keyEnd, err := strconv.Atoi(keyIdx[1])
	if err != nil {
		logrus.Panicf("key end %s is invalid in route %d", keyIdx[1], id)
	}
	if keyStart >= keyEnd {
		logrus.Panicf("key start %s >= key end %s in route %s", keyIdx[0], keyIdx[1], id)
	}

	return &Route{
		id:       id,
		operator: config.Operator,
		keyStart: keyStart,
		keyEnd:   keyEnd,
		value:    config.Value,
		upstream: config.Upstream,
	}
}

func (r *Route) Match(data []byte) bool {
	key := data[r.keyStart:r.keyEnd]

	return true
}
