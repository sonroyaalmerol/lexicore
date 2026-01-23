package authentik

import (
	"fmt"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

func init() {
	source.Register("authentik", func(raw map[string]any) (source.Source, error) {
		cfg, err := ParseConfig(raw)
		if err != nil {
			return nil, err
		}
		return NewAuthentikSource(cfg), nil
	})
}

func ParseConfig(raw map[string]any) (*Config, error) {
	url, _ := raw["url"].(string)
	token, _ := raw["token"].(string)

	if url == "" || token == "" {
		return nil, fmt.Errorf("authentik: 'url' and 'token' are required")
	}

	pageSize := int32(100)
	if ps, ok := raw["pageSize"].(float64); ok {
		pageSize = int32(ps)
	}

	return &Config{
		URL:      url,
		Token:    token,
		PageSize: pageSize,
	}, nil
}
