package transformer

import (
	"fmt"
	"math"
	"slices"

	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/utils"
	"github.com/huandu/go-clone"
)

type GroupAggregationTransformer struct {
	mappings       []GroupAggregationMapping
	includeParents bool
}

type GroupAggregationMapping struct {
	SourceAttribute string
	TargetAttribute string
	AggregationMode string
	DefaultValue    any
	WeightAttribute string
}

func NewGroupAggregationTransformer(config map[string]manifest.ConfigValue) (*GroupAggregationTransformer, error) {
	includeParents := false
	if v, ok := config["includeParents"]; ok {
		if b, ok := v.Value().(bool); ok {
			includeParents = b
		}
	}

	mappingsRaw, ok := config["mappings"]
	if !ok {
		return &GroupAggregationTransformer{
			mappings:       []GroupAggregationMapping{},
			includeParents: includeParents,
		}, nil
	}

	var mappings []GroupAggregationMapping

	switch v := mappingsRaw.Value().(type) {
	case []GroupAggregationMapping:
		mappings = v
	case []any:
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}

			mapping := GroupAggregationMapping{
				SourceAttribute: getString(m, "sourceAttribute"),
				TargetAttribute: getString(m, "targetAttribute"),
				AggregationMode: getString(m, "aggregationMode"),
				DefaultValue:    m["defaultValue"],
				WeightAttribute: getString(m, "weightAttribute"),
			}

			if mapping.AggregationMode == "" {
				mapping.AggregationMode = "first"
			}

			if !isValidAggregationMode(mapping.AggregationMode) {
				return nil, fmt.Errorf(
					"invalid aggregation mode '%s' for mapping %s -> %s",
					mapping.AggregationMode,
					mapping.SourceAttribute,
					mapping.TargetAttribute,
				)
			}

			mappings = append(mappings, mapping)
		}
	default:
		return nil, fmt.Errorf("invalid mappings configuration")
	}

	return &GroupAggregationTransformer{
		mappings:       mappings,
		includeParents: includeParents,
	}, nil
}

func (t *GroupAggregationTransformer) Transform(
	ctx *Context,
	identities map[string]source.Identity,
	allGroups map[string]source.Group,
) (map[string]source.Identity, map[string]source.Group, error) {
	for key, identity := range identities {
		if identity.Attributes == nil {
			identity.Attributes = make(map[string]any)
		}

		if len(identity.Groups) == 0 {
			for _, mapping := range t.mappings {
				if mapping.DefaultValue != nil {
					identity.Attributes[mapping.TargetAttribute] = mapping.DefaultValue
				}
			}

			identities[key] = identity
			continue
		}

		groups := t.resolveGroups(identity.Groups, allGroups)

		for _, mapping := range t.mappings {
			src := identity.Attributes[mapping.SourceAttribute]
			identity.Attributes[mapping.TargetAttribute] = t.applyMapping(src, groups, mapping)
		}

		identities[key] = identity
	}

	return identities, allGroups, nil
}

func (t *GroupAggregationTransformer) resolveGroups(
	groupNames []string,
	allGroups map[string]source.Group,
) []source.Group {
	if !t.includeParents {
		groups := make([]source.Group, 0, len(groupNames))
		for _, name := range groupNames {
			if g, ok := allGroups[name]; ok {
				groups = append(groups, g)
			}
		}
		return groups
	}

	seen := make(map[string]struct{})
	var result []source.Group

	var walk func(name string)
	walk = func(name string) {
		if _, visited := seen[name]; visited {
			return
		}
		seen[name] = struct{}{}

		g, ok := allGroups[name]
		if !ok {
			return
		}

		result = append(result, g)

		for _, parent := range g.Parents {
			walk(parent)
		}
	}

	for _, name := range groupNames {
		walk(name)
	}

	return result
}

func (t *GroupAggregationTransformer) applyMapping(
	existingAttr any,
	groups []source.Group,
	mapping GroupAggregationMapping,
) any {
	sourceKey := mapping.SourceAttribute

	newAttr := clone.Clone(existingAttr)

	switch mapping.AggregationMode {
	case "first":
		for _, group := range groups {
			if val, ok := group.Attributes[sourceKey]; ok {
				newAttr = val
				break
			}
		}
	case "last":
		for i := range len(groups) {
			idx := len(groups) - (i + 1)
			group := groups[idx]

			if val, ok := group.Attributes[sourceKey]; ok {
				newAttr = val
				break
			}
		}
	case "max":
		var maxVal any
		maxFloat := -math.MaxFloat64

		for _, group := range groups {
			v := group.Attributes[sourceKey]
			if f, ok := toFloat64(v); ok {
				if f > maxFloat {
					maxFloat = f
					maxVal = v
				}
			}
		}
		newAttr = maxVal
	case "min":
		var minVal any
		minFloat := math.MaxFloat64

		for _, group := range groups {
			v := group.Attributes[sourceKey]
			if f, ok := toFloat64(v); ok {
				if f < minFloat {
					minFloat = f
					minVal = v
				}
			}
		}
		newAttr = minVal
	case "append", "uniqueAppend":
		existingArr, isArr := existingAttr.([]any)
		if !isArr {
			existingArr = make([]any, 0, len(groups))
			if existingAttr != nil {
				existingArr = append(existingArr, existingAttr)
			}
		}

		for _, group := range groups {
			if groupVal, exists := group.Attributes[sourceKey]; exists {
				if mapping.AggregationMode == "uniqueAppend" {
					if groupValArr, isArr := groupVal.([]any); isArr {
						existingArr = utils.ConcatUnique(existingArr, groupValArr)
					} else {
						existingArr = utils.AppendUnique(existingArr, groupVal)
					}
				} else {
					if groupValArr, isArr := groupVal.([]any); isArr {
						existingArr = slices.Concat(existingArr, groupValArr)
					} else {
						existingArr = append(existingArr, groupVal)
					}
				}
			}
		}

		newAttr = existingArr
	case "weighted":
		newAttr = t.getWeightedValue(groups, mapping)
		if newAttr == nil && mapping.DefaultValue != nil {
			newAttr = mapping.DefaultValue
		}
	case "any":
		newAttr = false
		for _, group := range groups {
			v, hasV := group.Attributes[sourceKey]
			if !hasV {
				continue
			}

			if boolVal, ok := toBool(v); ok && boolVal {
				newAttr = true
				break
			}
		}
	case "all":
		newAttr = true
		for _, group := range groups {
			v, hasV := group.Attributes[sourceKey]
			if !hasV {
				continue
			}

			if boolVal, ok := toBool(v); ok && !boolVal {
				newAttr = false
				break
			}
		}
	case "anyFalse":
		newAttr = false
		for _, group := range groups {
			v, hasV := group.Attributes[sourceKey]
			if !hasV {
				continue
			}

			if boolVal, ok := toBool(v); ok && !boolVal {
				newAttr = true
				break
			}
		}
	case "allFalse":
		newAttr = true
		for _, group := range groups {
			v, hasV := group.Attributes[sourceKey]
			if !hasV {
				continue
			}

			if boolVal, ok := toBool(v); ok && boolVal {
				newAttr = false
				break
			}
		}
	default:
	}

	if newAttrArr, isArr := newAttr.([]any); isArr {
		if len(newAttrArr) == 0 {
			if mapping.DefaultValue != nil {
				newAttr = mapping.DefaultValue
			}
		}
	}

	if newAttr == nil && mapping.DefaultValue != nil {
		newAttr = mapping.DefaultValue
	}

	return newAttr
}

func (t *GroupAggregationTransformer) getWeightedValue(
	groups []source.Group,
	mapping GroupAggregationMapping,
) any {
	if mapping.WeightAttribute == "" {
		return nil
	}

	var mostWeighted any
	maxWeight := -math.MaxFloat64
	found := false

	for _, group := range groups {
		value, hasValue := group.Attributes[mapping.SourceAttribute]
		weight, hasWeight := group.Attributes[mapping.WeightAttribute]

		if !hasValue {
			continue
		}

		weightFloat := 0.0
		if hasWeight {
			if wf, ok := toFloat64(weight); ok {
				weightFloat = wf
			}
		}

		if !found || weightFloat > maxWeight {
			maxWeight = weightFloat
			mostWeighted = value
			found = true
		}
	}

	if !found {
		return nil
	}

	return mostWeighted
}

func isValidAggregationMode(mode string) bool {
	validModes := map[string]bool{
		"first":        true,
		"last":         true,
		"max":          true,
		"min":          true,
		"append":       true,
		"uniqueAppend": true,
		"weighted":     true,
		"any":          true,
		"all":          true,
		"anyFalse":     true,
		"allFalse":     true,
	}
	return validModes[mode]
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	default:
		return 0, false
	}
}

func toBool(v any) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case int:
		return val != 0, true
	case float64:
		return val != 0, true
	case string:
		if val == "true" || val == "1" {
			return true, true
		}
		if val == "false" || val == "0" || val == "" {
			return false, true
		}
	}
	return false, false
}
