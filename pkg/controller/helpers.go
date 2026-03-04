package controller

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"codeberg.org/lexicore/lexicore/pkg/source"
)

func (m *Manager) computeManifestHash(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("failed to marshal manifest: %w", err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

func (m *Manager) computeSourceDataHash(
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (string, error) {
	h := sha256.New()

	keys := make([]string, 0, len(identities)+len(groups))

	for k := range identities {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		jsonData, err := json.Marshal(identities[k])
		if err != nil {
			return "", fmt.Errorf("failed to marshal identity: %w", err)
		}
		h.Write([]byte(k))
		h.Write(jsonData)
	}

	keys = keys[:0]
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		jsonData, err := json.Marshal(groups[k])
		if err != nil {
			return "", fmt.Errorf("failed to marshal group: %w", err)
		}
		h.Write([]byte(k))
		h.Write(jsonData)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (m *Manager) hasSourceDataChanged(
	targetName string,
	currentHash string,
) bool {
	previousHash, exists := m.sourceDataHashes.Load(targetName)
	if !exists {
		return true
	}
	return previousHash != currentHash
}
