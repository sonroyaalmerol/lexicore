package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_Parse_IdentitySource(t *testing.T) {
	yaml := `
apiVersion: identity.orchestrator.io/v1
kind: IdentitySource
metadata:
  name: test-ldap
spec:
  type: ldap
  syncPeriod: 5m
  config:
    url: ldap://localhost:389
    bindDN: cn=admin,dc=example,dc=com
    bindPassword: password
    baseDN: dc=example,dc=com
`

	parser := NewParser()
	manifest, err := parser.Parse([]byte(yaml))
	require.NoError(t, err)

	source, ok := manifest.(*IdentitySource)
	require.True(t, ok)
	assert.Equal(t, "test-ldap", source.Name)
	assert.Equal(t, "ldap", source.Spec.Type)
	assert.Equal(t, "5m", source.Spec.SyncPeriod)
}

func TestParser_Parse_SyncTarget(t *testing.T) {
	yaml := `
apiVersion: identity.orchestrator.io/v1
kind: SyncTarget
metadata:
  name: test-sync
spec:
  sourceRef: test-ldap
  operator: dovecot
  dryRun: false
  transformers:
    - name: filter
      type: filter
      config:
        groupFilter: admins
  config:
    dsn: postgres://localhost/db
`

	parser := NewParser()
	manifest, err := parser.Parse([]byte(yaml))
	require.NoError(t, err)

	target, ok := manifest.(*SyncTarget)
	require.True(t, ok)
	assert.Equal(t, "test-sync", target.Name)
	assert.Equal(t, "test-ldap", target.Spec.SourceRef)
	assert.Equal(t, "dovecot", target.Spec.Operator)
	assert.Len(t, target.Spec.Transformers, 1)
}

func TestParser_Parse_InvalidKind(t *testing.T) {
	yaml := `
apiVersion: identity.orchestrator.io/v1
kind: InvalidKind
metadata:
  name: test
`

	parser := NewParser()
	_, err := parser.Parse([]byte(yaml))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown manifest kind")
}
