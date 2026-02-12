package operator

import (
	"bytes"
	"fmt"
	"html/template"
	"math"
	"sync"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"codeberg.org/lexicore/lexicore/pkg/utils"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type BaseOperator struct {
	name    string
	config  map[string]any
	mu      sync.RWMutex
	logger  *zap.Logger
	limiter *rate.Limiter

	groupAttributeMappings []GroupAttributeMapping
	attrPrefix             *string
}

func NewBaseOperator(name string, logger *zap.Logger) *BaseOperator {
	op := &BaseOperator{
		name:   name,
		logger: logger,
	}

	return op
}

func (b *BaseOperator) Name() string {
	return b.name
}

func (b *BaseOperator) GetConcurrency() int {
	workers := 10
	if w, ok := b.GetConfig("concurrency"); ok {
		if wInt, ok := w.(int); ok && wInt > 0 {
			workers = wInt
		} else if wFloat, ok := w.(float64); ok && wFloat > 0 {
			workers = int(wFloat)
		}
	}
	return workers
}

func (b *BaseOperator) GetLimiter() *rate.Limiter {
	if b.limiter != nil {
		return b.limiter
	}

	rateLimit := 50.0
	if rl, ok := b.GetConfig("rateLimit"); ok {
		if rlFloat, ok := rl.(float64); ok && rlFloat > 0 {
			rateLimit = rlFloat
		}
	}

	b.limiter = rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit))

	return b.limiter
}

func (b *BaseOperator) GetGroupAttributeMappings() []GroupAttributeMapping {
	if b.groupAttributeMappings != nil {
		return b.groupAttributeMappings
	}

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

	b.groupAttributeMappings = mappings

	return mappings
}

func (b *BaseOperator) EnrichIdentity(
	identity source.Identity,
	allGroups map[string]source.Group,
) source.Identity {
	enriched := identity

	if enriched.Attributes == nil {
		enriched.Attributes = make(map[string]any)
	}

	if len(b.GetGroupAttributeMappings()) == 0 {
		return enriched
	}

	for _, mapping := range b.GetGroupAttributeMappings() {
		b.applyMapping(&enriched, allGroups, mapping)
	}

	return enriched
}

func (b *BaseOperator) applyMapping(
	identity *source.Identity,
	allGroups map[string]source.Group,
	mapping GroupAttributeMapping,
) {
	targetKey := mapping.TargetAttribute

	if _, exists := identity.Attributes[targetKey]; exists {
		switch mapping.AggregationMode {
		case "append", "uniqueAppend":
			// Continue to append
		case "override":
			// Continue to override
		default:
			// Skip for other modes
			return
		}
	}

	values := make([]any, 0)

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

	case "last", "override":
		identity.Attributes[targetKey] = values[len(values)-1]

	case "max":
		identity.Attributes[targetKey] = getMaxValue(values)

	case "min":
		identity.Attributes[targetKey] = getMinValue(values)

	case "append", "uniqueAppend":
		existing, ok := identity.Attributes[targetKey].([]any)
		if !ok && identity.Attributes[targetKey] != nil {
			existing = []any{identity.Attributes[targetKey]}
		}

		if mapping.AggregationMode == "uniqueAppend" {
			seen := make(map[any]struct{}, len(existing)+len(values))

			for _, v := range existing {
				seen[v] = struct{}{}
			}

			for _, v := range values {
				if _, exists := seen[v]; !exists {
					existing = append(existing, v)
					seen[v] = struct{}{}
				}
			}
			identity.Attributes[targetKey] = existing
		} else {
			if existing == nil {
				existing = make([]any, 0, len(values))
			}
			identity.Attributes[targetKey] = append(existing, values...)
		}

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

	var mostWeighted any
	var maxWeight float64 = math.MinInt64

	for _, groupKey := range identity.Groups {
		group, ok := allGroups[groupKey]
		if !ok {
			continue
		}

		value, hasValue := group.Attributes[mapping.SourceAttribute]
		weight, hasWeight := group.Attributes[mapping.WeightAttribute]

		if !hasValue {
			continue
		}

		if !hasWeight {
			weight = 0
		}

		weightFloat, weightOk := toFloat64(weight)
		if !weightOk {
			weightFloat = 0
		}

		if weightFloat > maxWeight {
			maxWeight = weightFloat
			mostWeighted = value
		}
	}

	return mostWeighted
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

func (b *BaseOperator) GetTemplatedStringConfig(key string, attr map[string]any) (string, error) {
	str, err := b.GetStringConfig(key)
	if err != nil {
		return "", err
	}

	tmpl := template.New(key).
		Funcs(utils.CreateFuncMap()).
		Option("missingkey=zero")

	var buf bytes.Buffer

	parser, err := tmpl.Parse(str)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	err = parser.Execute(&buf, attr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	return buf.String(), nil
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
