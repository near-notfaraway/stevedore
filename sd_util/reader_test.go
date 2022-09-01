package sd_util

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

type TestStruct struct {
	T1 int
	T2 string
	T3 *struct{
		T4 bool
	}
}

// Should succeed unmarshal json file
func TestUnmarshalFile_Json(t *testing.T) {
	// build file
	jsonFile := "./tmp.json"
	f, err := os.OpenFile(jsonFile, os.O_WRONLY|os.O_CREATE, 0644)
	assert.Nil(t, err)
	_, err = f.WriteString("{" +
		"\"T1\": 1," +
		"\"T2\": \"2\"," +
		"\"T3\": {" +
		"\"T4\": true" +
		"}" +
		"}")
	assert.Nil(t, err)
	err = f.Close()
	assert.Nil(t, err)

	// check unmarshal result
	var config TestStruct
	err = UnmarshalFile(jsonFile, &config)
	assert.Nil(t, err)
	assert.Equal(t, 1, config.T1)
	assert.Equal(t, "2", config.T2)
	assert.Equal(t, true, config.T3.T4)

	// clean
	err = os.Remove(jsonFile)
	assert.Nil(t, err)
}
