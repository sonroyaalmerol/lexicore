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
