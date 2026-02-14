package transformer

import (
	"fmt"
	"regexp"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

type SelectorTransformer struct {
	selectors []Selector
}

type Selector interface {
	Apply(identity *source.Identity, allGroups map[string]source.Group) bool
}

func NewSelectorTransformer(config map[string]any) (*SelectorTransformer, error) {
	selectorsConfig, ok := config["selectors"].([]any)
	if !ok {
		return &SelectorTransformer{selectors: []Selector{}}, nil
	}

	selectors := make([]Selector, 0, len(selectorsConfig))
	for _, selectorConfig := range selectorsConfig {
		selectorMap, ok := selectorConfig.(map[string]any)
		if !ok {
			continue
		}

		selector, err := parseSelector(selectorMap)
		if err != nil {
			return nil, fmt.Errorf("failed to parse selector: %w", err)
		}
		selectors = append(selectors, selector)
	}

	return &SelectorTransformer{selectors: selectors}, nil
}

func (r *SelectorTransformer) Transform(
	ctx *Context,
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (map[string]source.Identity, map[string]source.Group, error) {
	for key := range identities {
		identity := identities[key]
		matched := false
		for _, selector := range r.selectors {
			if ok := selector.Apply(&identity, groups); ok {
				matched = true
				break
			}
		}
		if !matched {
			delete(identities, key)
		}
	}

	return identities, groups, nil
}

func parseSelector(config map[string]any) (Selector, error) {
	selectorType, ok := config["type"].(string)
	if !ok {
		return nil, fmt.Errorf("selector type not specified")
	}

	switch selectorType {
	case "regex":
		return parseRegexSelector(config)
	case "strict":
		return parseStrictSelector(config)
	case "group":
		return parseGroupSelector(config)
	default:
		return nil, fmt.Errorf("unknown selector type: %s", selectorType)
	}
}

type RegexSelector struct {
	field   string
	pattern *regexp.Regexp
}

func parseRegexSelector(config map[string]any) (*RegexSelector, error) {
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

	return &RegexSelector{
		field:   field,
		pattern: pattern,
	}, nil
}

func (r *RegexSelector) Apply(identity *source.Identity, _ map[string]source.Group) bool {
	var value string
	switch r.field {
	case "username":
		value = identity.Username
	case "email":
		value = identity.Email
	case "disabled":
		if identity.Disabled {
			value = "true"
		} else {
			value = "false"
		}
	default:
		if v, ok := identity.Attributes[r.field].(string); ok {
			value = v
		} else {
			return false
		}
	}

	return r.pattern.MatchString(value)
}

type StrictSelector struct {
	field string
	value string
}

func parseStrictSelector(config map[string]any) (*StrictSelector, error) {
	field, ok := config["field"].(string)
	if !ok {
		return nil, fmt.Errorf("field not specified")
	}

	patternStr, ok := config["value"].(string)
	if !ok {
		return nil, fmt.Errorf("value not specified")
	}

	return &StrictSelector{
		field: field,
		value: patternStr,
	}, nil
}

func (r *StrictSelector) Apply(identity *source.Identity, _ map[string]source.Group) bool {
	var value string
	switch r.field {
	case "username":
		value = identity.Username
	case "email":
		value = identity.Email
	case "disabled":
		if identity.Disabled {
			value = "true"
		} else {
			value = "false"
		}
	default:
		if v, ok := identity.Attributes[r.field].(string); ok {
			value = v
		} else {
			return false
		}
	}

	return r.value == value
}

type GroupSelector struct {
	field string
	value string
}

func parseGroupSelector(config map[string]any) (*GroupSelector, error) {
	field, ok := config["field"].(string)
	if !ok {
		return nil, fmt.Errorf("field not specified")
	}

	patternStr, ok := config["value"].(string)
	if !ok {
		return nil, fmt.Errorf("value not specified")
	}

	return &GroupSelector{
		field: field,
		value: patternStr,
	}, nil
}

func (r *GroupSelector) Apply(identity *source.Identity, allGroups map[string]source.Group) bool {
	for _, groupId := range identity.Groups {
		group, ok := allGroups[groupId]
		if !ok {
			continue
		}

		var value string
		switch r.field {
		case "name":
			value = group.Name
		case "gid":
			value = group.GID
		default:
			if v, ok := group.Attributes[r.field].(string); ok {
				value = v
			} else {
				return false
			}
		}

		if value == r.value {
			return true
		}
	}

	return false
}
