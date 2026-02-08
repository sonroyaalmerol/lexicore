package operator

import "fmt"

func getMaxValue(values []any) any {
	if len(values) == 0 {
		return nil
	}

	max := values[0]
	for _, v := range values[1:] {
		if compareValues(v, max) > 0 {
			max = v
		}
	}
	return max
}

func getMinValue(values []any) any {
	if len(values) == 0 {
		return nil
	}

	min := values[0]
	for _, v := range values[1:] {
		if compareValues(v, min) < 0 {
			min = v
		}
	}
	return min
}

func compareValues(a, b any) int {
	// Try numeric comparison first
	aFloat, aOk := toFloat64(a)
	bFloat, bOk := toFloat64(b)
	if aOk && bOk {
		if aFloat < bFloat {
			return -1
		}
		if aFloat > bFloat {
			return 1
		}
		return 0
	}

	// Fall back to string comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)
	if aStr < bStr {
		return -1
	}
	if aStr > bStr {
		return 1
	}
	return 0
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
	case uint:
		return float64(val), true
	case uint64:
		return float64(val), true
	case uint32:
		return float64(val), true
	default:
		return 0, false
	}
}
