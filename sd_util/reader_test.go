package sd_util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

type testStruct struct {
	T1 int
	T2 string
	T3 *struct {
		T4 bool
	}
}

func TestUnmarshalFileConvey(t *testing.T) {
	convey.Convey("Test UnmarshalFile", t, func() {
		convey.Convey("reads and decodes JSON", func() {
			jsonFile := filepath.Join(t.TempDir(), "config.json")
			err := os.WriteFile(jsonFile, []byte(`{
				"T1": 1,
				"T2": "2",
				"T3": {
					"T4": true
				}
			}`), 0600)
			convey.So(err, convey.ShouldBeNil)

			var config testStruct
			err = UnmarshalFile(jsonFile, &config)
			convey.So(err, convey.ShouldBeNil)
			convey.So(config.T1, convey.ShouldEqual, 1)
			convey.So(config.T2, convey.ShouldEqual, "2")
			convey.So(config.T3, convey.ShouldNotBeNil)
			convey.So(config.T3.T4, convey.ShouldBeTrue)
		})

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
