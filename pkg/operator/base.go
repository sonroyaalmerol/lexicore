package operator

import (
	"fmt"
	"sync"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"go.uber.org/zap"
)

type BaseOperator struct {
	name   string
	config map[string]any
	mu     sync.RWMutex
	logger *zap.Logger

	groupAttributeMappings []GroupAttributeMapping
}

func NewBaseOperator(name string, logger *zap.Logger) *BaseOperator {
	op := &BaseOperator{
		name:   name,
		logger: logger,
	}
	op.groupAttributeMappings = op.getGroupAttributeMappings()

	return op
}

func (b *BaseOperator) Name() string {
	return b.name
}

func (b *BaseOperator) getGroupAttributeMappings() []GroupAttributeMapping {
	mappingsRaw, ok := b.GetConfig("groupAttributeMappings")
	if !ok {
		return nil
	}

	var mappings []GroupAttributeMapping

	switch v := mappingsRaw.(type) {
	case []GroupAttributeMapping:
		return v
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				mapping := GroupAttributeMapping{
					SourceAttribute: getString(m, "sourceAttribute"),
					TargetAttribute: getString(m, "targetAttribute"),
					AggregationMode: getString(m, "aggregationMode"),
					DefaultValue:    m["defaultValue"],
					WeightAttribute: getString(m, "weightAttribute"),
				}
				mappings = append(mappings, mapping)
			}
		}
	}

	return mappings
}

func (b *BaseOperator) GetAttributePrefix() string {
	if prefix, err := b.GetStringConfig("attributePrefix"); err == nil {
		return prefix
	}

	return ""
}

func (b *BaseOperator) EnrichIdentity(
	identity source.Identity,
	allGroups map[string]source.Group,
) source.Identity {
	enriched := identity

	if enriched.Attributes == nil {
		enriched.Attributes = make(map[string]any)
	}

	if len(b.groupAttributeMappings) == 0 {
		return enriched
	}

	for _, mapping := range b.groupAttributeMappings {
		b.applyMapping(&enriched, allGroups, mapping)
	}

	return enriched
}

func (b *BaseOperator) applyMapping(
	identity *source.Identity,
	allGroups map[string]source.Group,
	mapping GroupAttributeMapping,
) {
	targetKey := b.GetAttributePrefix() + mapping.TargetAttribute

	// Skip if attribute already exists and we're not appending
	if _, exists := identity.Attributes[targetKey]; exists && mapping.AggregationMode != "append" {
		return
	}

	values := make([]any, 0, len(identity.Groups))

	for _, groupKey := range identity.Groups {
		group, ok := allGroups[groupKey]
		if !ok {
			continue
		}

		if value, ok := group.Attributes[mapping.SourceAttribute]; ok {
			if arrVal, isArr := value.([]any); isArr {
				values = append(values, arrVal...)
			} else {
				values = append(values, value)
			}
		}
	}

	if len(values) == 0 {
		if mapping.DefaultValue != nil {
			identity.Attributes[targetKey] = mapping.DefaultValue
		}
		return
	}

	switch mapping.AggregationMode {
	case "first":
		identity.Attributes[targetKey] = values[0]

	case "last":
		identity.Attributes[targetKey] = values[len(values)-1]

	case "max":
		identity.Attributes[targetKey] = getMaxValue(values)

	case "min":
		identity.Attributes[targetKey] = getMinValue(values)

	case "append":
		existing, _ := identity.Attributes[targetKey].([]any)
		if existing == nil {
			existing = make([]any, 0, len(values))
		}
		identity.Attributes[targetKey] = append(existing, values...)

	case "override":
		identity.Attributes[targetKey] = values[len(values)-1]

	case "weighted":
		identity.Attributes[targetKey] = b.getWeightedValue(identity, allGroups, mapping)

	default:
		identity.Attributes[targetKey] = values[0]
	}
}

func (b *BaseOperator) getWeightedValue(
	identity *source.Identity,
	allGroups map[string]source.Group,
	mapping GroupAttributeMapping,
) any {
	if mapping.WeightAttribute == "" {
		return nil
	}

	var totalWeight float64
	var weightedSum float64

	for _, groupKey := range identity.Groups {
		group, ok := allGroups[groupKey]
		if !ok {
			continue
		}

		value, hasValue := group.Attributes[mapping.SourceAttribute]
		weight, hasWeight := group.Attributes[mapping.WeightAttribute]

		if !hasValue || !hasWeight {
			continue
		}

		valueFloat, valueOk := toFloat64(value)
		weightFloat, weightOk := toFloat64(weight)

		if valueOk && weightOk {
			weightedSum += valueFloat * weightFloat
			totalWeight += weightFloat
		}
	}

	if totalWeight == 0 {
		return mapping.DefaultValue
	}

	return weightedSum / totalWeight
}

func (b *BaseOperator) GetConfig(key string) (any, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	val, ok := b.config[key]
	return val, ok
}

func (b *BaseOperator) GetStringConfig(key string) (string, error) {
	val, ok := b.GetConfig(key)
	if !ok {
		return "", fmt.Errorf("config key %s not found", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("config key %s is not a string", key)
	}
	return str, nil
}

func (b *BaseOperator) SetConfig(config map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.config = config
}

func (b *BaseOperator) LogInfo(s string, v ...any) {
	if b.logger != nil {
		b.logger.Info(fmt.Sprintf(s, v...), zap.String("operator", b.name))
	}
}

func (b *BaseOperator) LogWarn(s string, v ...any) {
	if b.logger != nil {
		b.logger.Warn(fmt.Sprintf(s, v...), zap.String("operator", b.name))
	}
}

func (b *BaseOperator) LogError(err error) {
	if err != nil && b.logger != nil {
		b.logger.Error(err.Error(), zap.String("operator", b.name))
	}
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
