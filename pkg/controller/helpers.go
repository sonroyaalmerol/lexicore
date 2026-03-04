package controller

import (
	"fmt"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/gohugoio/hashstructure"
)

func (m *Manager) computeManifestHash(v any) (string, error) {
	manifestHash, err := hashstructure.Hash(v, nil)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", manifestHash), nil
}

func (m *Manager) computeSourceDataHash(
	identities map[string]source.Identity,
	groups map[string]source.Group,
) (string, error) {
	identitiesHash, err := hashstructure.Hash(identities, nil)
	if err != nil {
		return "", err
	}
	groupsHash, err := hashstructure.Hash(groups, nil)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d", identitiesHash+groupsHash), nil
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
