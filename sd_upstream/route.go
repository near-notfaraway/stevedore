package sd_upstream

import "github.com/near-notfaraway/stevedore/sd_config"

type RouteOperator interface {
	Perform(leftVal, rightVal interface{}) bool
}

type Route struct {
	operator string
	bytes    string
	value    string
	upstream string
}

func NewRoute(config sd_config.RouteConfig) *Route {
	return &Route{
		operator: config.Operator,
		bytes:    config.Bytes,
		value:    config.Value,
		upstream: config.Upstream,
	}
}
