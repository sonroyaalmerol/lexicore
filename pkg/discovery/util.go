package discovery

import (
	"os"
)

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "node-unknown"
	}
	return hostname
}
