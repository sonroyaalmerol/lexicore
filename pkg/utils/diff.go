package utils

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
