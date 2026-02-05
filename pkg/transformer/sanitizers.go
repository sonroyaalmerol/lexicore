package transformer

import (
	"fmt"
	"regexp"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type SanitizerTransformer struct {
	sanitizers []Sanitizer
}

type Sanitizer interface {
	Apply(identity *source.Identity) error
}

func NewSanitizerTransformer(config map[string]any) (*SanitizerTransformer, error) {
	sanitizersConfig, ok := config["sanitizers"].([]any)
	if !ok {
		return &SanitizerTransformer{sanitizers: []Sanitizer{}}, nil
	}

	sanitizers := make([]Sanitizer, 0, len(sanitizersConfig))
	for _, sanitizerConfig := range sanitizersConfig {
		sanitizerMap, ok := sanitizerConfig.(map[string]any)
		if !ok {
			continue
		}

		sanitizer, err := parseSanitizer(sanitizerMap)
		if err != nil {
			return nil, fmt.Errorf("failed to parse sanitizer: %w", err)
		}
		sanitizers = append(sanitizers, sanitizer)
	}

	return &SanitizerTransformer{sanitizers: sanitizers}, nil
}

func (r *SanitizerTransformer) Transform(
	ctx *Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (map[string]source.Identity, map[string]source.Group, error) {
	for key := range identities {
		identity := identities[key]
		for _, sanitizer := range r.sanitizers {
			if err := sanitizer.Apply(&identity); err != nil {
				return nil, nil, fmt.Errorf("sanitizer application failed: %w", err)
			}
		}
		identities[key] = identity
	}

	return identities, groups, nil
}

func parseSanitizer(config map[string]any) (Sanitizer, error) {
	sanitizerType, ok := config["type"].(string)
	if !ok {
		return nil, fmt.Errorf("sanitizer type not specified")
	}

	switch sanitizerType {
	case "regex":
		return parseRegexSanitizer(config)
	case "normalize":
		return parseNormalizeSanitizer(config)
	case "compute":
		return parseComputeSanitizer(config)
	default:
		return nil, fmt.Errorf("unknown sanitizer type: %s", sanitizerType)
	}
}

type RegexSanitizer struct {
	field   string
	pattern *regexp.Regexp
	replace string
}

func parseRegexSanitizer(config map[string]any) (*RegexSanitizer, error) {
	field, ok := config["field"].(string)
	if !ok {
		return nil, fmt.Errorf("field not specified")
	}

	patternStr, ok := config["pattern"].(string)
	if !ok {
		return nil, fmt.Errorf("pattern not specified")
	}

	pattern, err := regexp.Compile(patternStr)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	replace, _ := config["replace"].(string)

	return &RegexSanitizer{
		field:   field,
		pattern: pattern,
		replace: replace,
	}, nil
}

func (r *RegexSanitizer) Apply(identity *source.Identity) error {
	var value string
	switch r.field {
	case "username":
		value = identity.Username
	case "email":
		value = identity.Email
	default:
		if v, ok := identity.Attributes[r.field].(string); ok {
			value = v
		} else {
			return nil
		}
	}

	newValue := r.pattern.ReplaceAllString(value, r.replace)

	switch r.field {
	case "username":
		identity.Username = newValue
	case "email":
		identity.Email = newValue
	default:
		identity.Attributes[r.field] = newValue
	}

	return nil
}

type NormalizeSanitizer struct {
	field     string
	operation string
}

func parseNormalizeSanitizer(config map[string]any) (*NormalizeSanitizer, error) {
	field, ok := config["field"].(string)
	if !ok {
		return nil, fmt.Errorf("field not specified")
	}

	operation, ok := config["operation"].(string)
	if !ok {
		return nil, fmt.Errorf("operation not specified")
	}

	return &NormalizeSanitizer{
		field:     field,
		operation: operation,
	}, nil
}

func (r *NormalizeSanitizer) Apply(identity *source.Identity) error {
	var value string
	switch r.field {
	case "username":
		value = identity.Username
	case "email":
		value = identity.Email
	default:
		if v, ok := identity.Attributes[r.field].(string); ok {
			value = v
		} else {
			return nil
		}
	}

	var newValue string
	switch r.operation {
	case "lowercase":
		newValue = strings.ToLower(value)
	case "uppercase":
		newValue = strings.ToUpper(value)
	case "trim":
		newValue = strings.TrimSpace(value)
	default:
		return fmt.Errorf("unknown operation: %s", r.operation)
	}

	switch r.field {
	case "username":
		identity.Username = newValue
	case "email":
		identity.Email = newValue
	default:
		identity.Attributes[r.field] = newValue
	}

	return nil
}

type ComputeSanitizer struct {
	target     string
	expression string
}

func parseComputeSanitizer(config map[string]any) (*ComputeSanitizer, error) {
	target, ok := config["target"].(string)
	if !ok {
		return nil, fmt.Errorf("target not specified")
	}

	expression, ok := config["expression"].(string)
	if !ok {
		return nil, fmt.Errorf("expression not specified")
	}

	return &ComputeSanitizer{
		target:     target,
		expression: expression,
	}, nil
}

func (r *ComputeSanitizer) Apply(identity *source.Identity) error {
	result := r.evaluateExpression(identity, r.expression)
	identity.Attributes[r.target] = result
	return nil
}

func (r *ComputeSanitizer) evaluateExpression(identity *source.Identity, expr string) string {
	var sb strings.Builder
	sb.Grow(len(expr) + 32)

	result := expr
	result = strings.ReplaceAll(result, "{{username}}", identity.Username)
	result = strings.ReplaceAll(result, "{{email}}", identity.Email)

	for key, value := range identity.Attributes {
		if str, ok := value.(string); ok {
			placeholder := "{{" + key + "}}"
			result = strings.ReplaceAll(result, placeholder, str)
		}
	}

	return result
}
