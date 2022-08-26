package sd_util

import (
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/stretchr/testify/assert"
	"testing"
)

// should succeed unmarshal json file
func TestUnmarshalFile_Json(t *testing.T) {
	jsonFile := "../sd_config/config.example.json"
	var config sd_config.Config
	err := UnmarshalFile(jsonFile, &config)
	assert.Nil(t, err)
	assert.Equal(t, "0.0.0.0:2614", config.Server.ListenAddr)
	assert.Equal(t, 4, config.Server.ListenParallel)
}
