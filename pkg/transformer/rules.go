package transformer

import (
	"fmt"
	"regexp"
	"strings"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type RuleTransformer struct {
	rules []Rule
}

type Rule interface {
	Apply(identity *source.Identity) error
}

func NewRuleTransformer(config map[string]any) (*RuleTransformer, error) {
	rt := &RuleTransformer{}

	if rulesConfig, ok := config["rules"].([]any); ok {
		for _, ruleConfig := range rulesConfig {
			ruleMap, ok := ruleConfig.(map[string]any)
			if !ok {
				continue
			}

			rule, err := parseRule(ruleMap)
			if err != nil {
				return nil, fmt.Errorf("failed to parse rule: %w", err)
			}
			rt.rules = append(rt.rules, rule)
		}
	}

	return rt, nil
}

func (r *RuleTransformer) Transform(
	ctx *Context,
	identities []source.Identity,
	groups []source.Group,
) ([]source.Identity, []source.Group, error) {
	for i := range identities {
		for _, rule := range r.rules {
			if err := rule.Apply(&identities[i]); err != nil {
				return nil, nil, fmt.Errorf("rule application failed: %w", err)
			}
		}
	}

	return identities, groups, nil
}

func parseRule(config map[string]any) (Rule, error) {
	ruleType, ok := config["type"].(string)
	if !ok {
		return nil, fmt.Errorf("rule type not specified")
	}

	switch ruleType {
	case "regex":
		return parseRegexRule(config)
	case "normalize":
		return parseNormalizeRule(config)
	case "compute":
		return parseComputeRule(config)
	default:
		return nil, fmt.Errorf("unknown rule type: %s", ruleType)
	}
}

type RegexRule struct {
	field   string
	pattern *regexp.Regexp
	replace string
}

func parseRegexRule(config map[string]any) (*RegexRule, error) {
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

	return &RegexRule{
		field:   field,
		pattern: pattern,
		replace: replace,
	}, nil
}

func (r *RegexRule) Apply(identity *source.Identity) error {
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
			return nil // Field not found or not a string
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

type NormalizeRule struct {
	field     string
	operation string // lowercase, uppercase, trim
}

func parseNormalizeRule(config map[string]any) (*NormalizeRule, error) {
	field, ok := config["field"].(string)
	if !ok {
		return nil, fmt.Errorf("field not specified")
	}

	operation, ok := config["operation"].(string)
	if !ok {
		return nil, fmt.Errorf("operation not specified")
	}

	return &NormalizeRule{
		field:     field,
		operation: operation,
	}, nil
}

func (r *NormalizeRule) Apply(identity *source.Identity) error {
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

type ComputeRule struct {
	target     string
	expression string
}

func parseComputeRule(config map[string]any) (*ComputeRule, error) {
	target, ok := config["target"].(string)
	if !ok {
		return nil, fmt.Errorf("target not specified")
	}

	expression, ok := config["expression"].(string)
	if !ok {
		return nil, fmt.Errorf("expression not specified")
	}

	return &ComputeRule{
		target:     target,
		expression: expression,
	}, nil
}

func (r *ComputeRule) Apply(identity *source.Identity) error {
	// TODO: use a proper expression evaluator
	result := r.evaluateExpression(identity, r.expression)
	identity.Attributes[r.target] = result
	return nil
}

func (r *ComputeRule) evaluateExpression(identity *source.Identity, expr string) string {
	// TODO: Replace {{field}} with actual values
	result := expr
	result = strings.ReplaceAll(result, "{{username}}", identity.Username)
	result = strings.ReplaceAll(result, "{{email}}", identity.Email)

	for key, value := range identity.Attributes {
		if str, ok := value.(string); ok {
			result = strings.ReplaceAll(result, fmt.Sprintf("{{%s}}", key), str)
		}
	}

	return result
}
