package utils

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

