package manifest

import (
	"fmt"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

type ConfigValue struct {
	raw any
}

func NewConfigValue(raw any) ConfigValue {
	return ConfigValue{raw: raw}
}

func (c *ConfigValue) UnmarshalYAML(unmarshal func(any) error) error {
	var secret SecretValue
	if err := unmarshal(&secret); err == nil && secret.ValueFrom != nil {
		resolved, err := resolveValueFrom(secret.ValueFrom)
		if err != nil {
			return err
		}
		c.raw = resolved
		return nil
	}

	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	c.raw = raw
	return nil
}

func (c ConfigValue) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(c.raw)
}

func (c ConfigValue) Value() any {
	return c.raw
}

func (c ConfigValue) String() string {
	if c.raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", c.raw))
}

func resolveValueFrom(src *ValueFromSource) (string, error) {
	switch {
	case src.Env != "":
		val, ok := os.LookupEnv(src.Env)
		if !ok {
			return "", fmt.Errorf("env var %q not found", src.Env)
		}
		return val, nil

	case src.File != "":
		data, err := os.ReadFile(src.File)
		if err != nil {
			return "", fmt.Errorf("failed to read secret file %q: %w", src.File, err)
		}
		return string(data), nil

	default:
		return "", fmt.Errorf("valueFrom requires either env or file")
	}
}
