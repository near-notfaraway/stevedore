package sd_util

import (
	"encoding/json"
	"fmt"
	"github.com/tidwall/jsonc"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"strings"
)

const (
	FilePostfixJson = "json"
	FilePostYaml    = "yaml"
)

func UnmarshalFile(filename string, v interface{}) error {
	// read file
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read file %s failed: %w", filename, err)
	}

	// get file postfix
	nameSli := strings.Split(filename, ".")
	postfix := nameSli[len(nameSli)-1]

	// unmarshal file according to postfix
	switch postfix {
	case FilePostfixJson:
		err = json.Unmarshal(jsonc.ToJSONInPlace(buf), v)
		if err != nil {
			return fmt.Errorf("file %s json unmarshal failed: %w", filename, err)
		}

	case FilePostYaml:
		err = yaml.Unmarshal(buf, v)
		if err != nil {
			return fmt.Errorf("file %s yaml unmarshal failed: %w", filename, err)
		}

	default:
		return fmt.Errorf("file %s has invalid postfix", filename)
	}

	return nil
}
