package utils

import (
	"fmt"
	"strings"
)

func StringArrDiff(oldArr, newArr []string) (added, deleted []string) {
	oldSet := make(map[string]bool)
	newSet := make(map[string]bool)

	for _, s := range oldArr {
		oldSet[s] = true
	}
	for _, s := range newArr {
		newSet[s] = true
	}

	for _, s := range oldArr {
		if !newSet[s] {
			deleted = append(deleted, s)
		}
	}

	for _, s := range newArr {
		if !oldSet[s] {
			added = append(added, s)
		}
	}

	return added, deleted
}

func DiffString(old string, newS string) string {
	return fmt.Sprintf("%s -> %s", old, newS)
}

func DiffArrString(oldArr, newArr []string) string {
	added, deleted := StringArrDiff(oldArr, newArr)

	return fmt.Sprintf("ADD: %s | REMOVE: %s", strings.Join(added, ","), strings.Join(deleted, ","))
}
