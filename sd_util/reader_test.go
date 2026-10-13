package sd_util

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

type testStruct struct {
	T1 int
	T2 string
	T3 *struct {
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
	var config testStruct
	err = UnmarshalFile(jsonFile, &config)
	assert.Nil(t, err)
	assert.Equal(t, 1, config.T1)
	assert.Equal(t, "2", config.T2)
	assert.Equal(t, true, config.T3.T4)

	// clean
	err = os.Remove(jsonFile)
	assert.Nil(t, err)
}

func TestUnmarshalFileConvey(t *testing.T) {
	convey.Convey("Test UnmarshalFile", t, func() {
		convey.Convey("reads and decodes mocked JSON", func() {
			patches := gomonkey.ApplyFuncReturn(
				ioutil.ReadFile,
				[]byte(`{"T1":7,"T2":"mocked"}`),
				nil,
			)
			defer patches.Reset()

			var got testStruct
			err := UnmarshalFile("mock.json", &got)
			convey.So(err, convey.ShouldBeNil)
			convey.So(got.T1, convey.ShouldEqual, 7)
			convey.So(got.T2, convey.ShouldEqual, "mocked")
		})
	})
}
