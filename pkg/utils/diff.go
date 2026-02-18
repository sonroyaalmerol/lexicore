package utils

import (
	"fmt"
	"strings"
)

func StringArrDiff(oldArr, newArr []string) (added, deleted []string) {
	oldSet := make(map[string]struct{}, len(oldArr))
	newSet := make(map[string]struct{}, len(newArr))

	for _, s := range oldArr {
		oldSet[s] = struct{}{}
	}
	for _, s := range newArr {
		newSet[s] = struct{}{}
	}

	deleted = make([]string, 0, len(oldArr))
	for _, s := range oldArr {
		if _, exists := newSet[s]; !exists {
			deleted = append(deleted, s)
		}
	}

	added = make([]string, 0, len(newArr))
	for _, s := range newArr {
		if _, exists := oldSet[s]; !exists {
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
	var toRet strings.Builder
	for i, add := range added {
		toRet.WriteRune('+')
		toRet.WriteString(add)
		if i < len(added)-1 {
			toRet.WriteRune(' ')
		}
	}

	if len(added) > 0 && len(deleted) > 0 {
		toRet.WriteRune(' ')
	}

	for i, del := range deleted {
		toRet.WriteRune('-')
		toRet.WriteString(del)
		if i < len(deleted)-1 {
			toRet.WriteRune(' ')
		}
	}

	return toRet.String()
}
