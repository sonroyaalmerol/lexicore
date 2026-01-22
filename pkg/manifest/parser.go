package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Parser struct {
	validator *Validator
}

func NewParser() *Parser {
	return &Parser{
		validator: NewValidator(),
	}
}

func (p *Parser) ParseFile(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return p.Parse(data)
}

func (p *Parser) Parse(data []byte) (any, error) {
	expanded := os.ExpandEnv(string(data))

	var typeMeta TypeMeta
	if err := yaml.Unmarshal([]byte(expanded), &typeMeta); err != nil {
		return nil, fmt.Errorf("failed to parse type metadata: %w", err)
	}

	var manifest any
	switch typeMeta.Kind {
	case "IdentitySource":
		var source IdentitySource
		if err := yaml.Unmarshal([]byte(expanded), &source); err != nil {
			return nil, fmt.Errorf("failed to parse IdentitySource: %w", err)
		}
		manifest = &source
	case "SyncTarget":
		var target SyncTarget
		if err := yaml.Unmarshal([]byte(expanded), &target); err != nil {
			return nil, fmt.Errorf("failed to parse SyncTarget: %w", err)
		}
		manifest = &target
	default:
		return nil, fmt.Errorf("unknown manifest kind: %s", typeMeta.Kind)
	}

	if err := p.validator.Validate(manifest); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return manifest, nil
}

func (p *Parser) ParseDirectory(dir string) ([]any, error) {
	var manifests []any

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || (!strings.HasSuffix(path, ".yaml") &&
			!strings.HasSuffix(path, ".yml")) {
			return nil
		}

		manifest, err := p.ParseFile(path)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}

		manifests = append(manifests, manifest)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return manifests, nil
}

type TypeMeta struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind"`
}
