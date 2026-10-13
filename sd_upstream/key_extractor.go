package sd_upstream

import (
	"fmt"

	"github.com/near-notfaraway/stevedore/sd_config"
)

const (
	keyExtractorTypeFixed       = "fixed"
	keyExtractorTypeLengthField = "length_field"
)

type keyExtractor interface {
	Extract(data []byte) ([]byte, bool)
}

func newKeyExtractor(config *sd_config.UpstreamConfig) (keyExtractor, error) {
	if config.KeyExtractor == nil {
		return nil, fmt.Errorf("key extractor config is required")
	}

	switch config.KeyExtractor.Type {
	case keyExtractorTypeFixed:
		if config.KeyExtractor.Fixed == nil {
			return nil, fmt.Errorf("fixed key extractor config is required")
		}
		if config.KeyExtractor.LengthField != nil {
			return nil, fmt.Errorf("length field config is invalid for fixed key extractor")
		}
		return newFixedKeyExtractor(config.KeyExtractor.Fixed.KeyBytes, config.KeyExtractor.Fixed.KeyMask)
	case keyExtractorTypeLengthField:
		if config.KeyExtractor.LengthField == nil {
			return nil, fmt.Errorf("length field key extractor config is required")
		}
		if config.KeyExtractor.Fixed != nil {
			return nil, fmt.Errorf("fixed config is invalid for length field key extractor")
		}
		return newLengthFieldKeyExtractor(*config.KeyExtractor.LengthField)
	case "":
		return nil, fmt.Errorf("key extractor type is required")
	default:
		return nil, fmt.Errorf("key extractor type %q is invalid", config.KeyExtractor.Type)
	}
}
