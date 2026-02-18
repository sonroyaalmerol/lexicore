package utils

import (
	"slices"
	"strings"
)

func SlicesAreEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	counts := make(map[string]int)

	for _, s := range a {
		counts[s]++
	}

	for _, s := range b {
		counts[s]--
		if counts[s] < 0 {
			return false
		}
	}

	return true
}

func SliceToSortedString(a []string, delimiter string) string {
	sorted := slices.Clone(a)
	slices.Sort(sorted)

	return strings.Join(sorted, delimiter)
}

func ConcatUnique[T comparable](slices ...[]T) []T {
	totalLen := 0
	for _, s := range slices {
		totalLen += len(s)
	}

	result := make([]T, 0, totalLen)
	seen := make(map[T]struct{}, totalLen)

	for _, slice := range slices {
		for _, val := range slice {
			if _, exists := seen[val]; !exists {
				seen[val] = struct{}{}
				result = append(result, val)
			}
		}
	}

	return result
}

func AppendUnique[T comparable](slice []T, element T) []T {
	if slices.Contains(slice, element) {
		return slice
	}
	return append(slice, element)
}
