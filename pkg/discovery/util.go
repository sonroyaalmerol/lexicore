package discovery

import (
	"os"
	"strings"
)

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

func parseSeeds(seedStr string) []string {
	if seedStr == "" {
		return []string{}
	}

	seeds := strings.Split(seedStr, ",")
	result := make([]string, 0, len(seeds))

	for _, seed := range seeds {
		trimmed := strings.TrimSpace(seed)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "node-unknown"
	}
	return hostname
}
