package sd_util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
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
	jsonFile := filepath.Join(t.TempDir(), "config.json")
	err := os.WriteFile(jsonFile, []byte(`{
		"T1": 1,
		"T2": "2",
		"T3": {
			"T4": true
		}
	}`), 0600)
	require.NoError(t, err)

	var config testStruct
	err = UnmarshalFile(jsonFile, &config)
	require.NoError(t, err)
	require.Equal(t, 1, config.T1)
	require.Equal(t, "2", config.T2)
	require.NotNil(t, config.T3)
	require.True(t, config.T3.T4)
}

func TestUnmarshalFileConvey(t *testing.T) {
	convey.Convey("Test UnmarshalFile", t, func() {
		convey.Convey("reads and decodes JSON with comments", func() {
			jsonFile := filepath.Join(t.TempDir(), "config.json")
			err := os.WriteFile(
				jsonFile,
				[]byte(`{"T1":7,"T2":"jsonc" /* comment */}`),
				0600,
			)
			convey.So(err, convey.ShouldBeNil)

			var got testStruct
			err = UnmarshalFile(jsonFile, &got)
			convey.So(err, convey.ShouldBeNil)
			convey.So(got.T1, convey.ShouldEqual, 7)
			convey.So(got.T2, convey.ShouldEqual, "jsonc")
		})
	})
}
