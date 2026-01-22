package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_Parse_IdentitySource(t *testing.T) {
	yaml := `
apiVersion: lexicore.io/v1
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
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: test-sync
spec:
  sourceRef: test-ldap
  operator: dovecot
  dryRun: false
  transformers:
    - name: selector
      type: selector
      config:
        groupSelector: admins
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
apiVersion: lexicore.io/v1
kind: InvalidKind
metadata:
  name: test
`

	parser := NewParser()
	_, err := parser.Parse([]byte(yaml))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown manifest kind")
}

func TestParser_ValidateAPIVersion(t *testing.T) {
	tests := []struct {
		name        string
		manifest    string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid api version",
			manifest: `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: test
spec:
  sourceRef: source
  operator: mock
`,
			expectError: false,
		},
		{
			name: "missing api version",
			manifest: `
kind: SyncTarget
metadata:
  name: test
spec:
  sourceRef: source
  operator: mock
`,
			expectError: true,
			errorMsg:    "apiVersion is required",
		},
		{
			name: "invalid api version",
			manifest: `
apiVersion: lexicore.io/v2
kind: SyncTarget
metadata:
  name: test
spec:
  sourceRef: source
  operator: mock
`,
			expectError: true,
			errorMsg:    "unsupported apiVersion",
		},
		{
			name: "wrong api group",
			manifest: `
apiVersion: wronggroup.io/v1
kind: SyncTarget
metadata:
  name: test
spec:
  sourceRef: source
  operator: mock
`,
			expectError: true,
			errorMsg:    "unsupported apiVersion",
		},
	}

	parser := NewParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse([]byte(tt.manifest))

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParser_ValidateKind(t *testing.T) {
	tests := []struct {
		name        string
		manifest    string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid SyncTarget kind",
			manifest: `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: test
spec:
  sourceRef: source
  operator: mock
`,
			expectError: false,
		},
		{
			name: "valid IdentitySource kind",
			manifest: `
apiVersion: lexicore.io/v1
kind: IdentitySource
metadata:
  name: test
spec:
  type: ldap
  config:
    url: ldap://localhost
    bindDN: cn=admin,dc=example,dc=com
    bindPassword: secret
    baseDN: ou=users,dc=example,dc=com
`,
			expectError: false,
		},
		{
			name: "unknown kind",
			manifest: `
apiVersion: lexicore.io/v1
kind: UnknownKind
metadata:
  name: test
spec:
  foo: bar
`,
			expectError: true,
			errorMsg:    "unknown manifest kind",
		},
	}

	parser := NewParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse([]byte(tt.manifest))

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParser_EnvironmentVariableExpansion(t *testing.T) {
	os.Setenv("TEST_DOMAIN", "test.example.com")
	os.Setenv("TEST_QUOTA", "5000")
	defer os.Unsetenv("TEST_DOMAIN")
	defer os.Unsetenv("TEST_QUOTA")

	manifest := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: test
spec:
  sourceRef: source
  operator: mock
  transformers:
    - name: test
      type: constant
      config:
        mappings:
          domain: ${TEST_DOMAIN}
          quota: ${TEST_QUOTA}
`

	parser := NewParser()
	parsed, err := parser.Parse([]byte(manifest))
	require.NoError(t, err)

	target, ok := parsed.(*SyncTarget)
	require.True(t, ok)

	mappings := target.Spec.Transformers[0].Config["mappings"].(map[string]any)
	assert.Equal(t, "test.example.com", mappings["domain"])
	assert.Equal(t, 5000, int(mappings["quota"].(uint64)))
}

func TestParser_ParseFile(t *testing.T) {
	tmpDir := t.TempDir()

	manifestContent := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: file-test
spec:
  sourceRef: source
  operator: mock
`

	manifestPath := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(manifestPath, []byte(manifestContent), 0644)
	require.NoError(t, err)

	parser := NewParser()
	parsed, err := parser.ParseFile(manifestPath)
	require.NoError(t, err)

	target, ok := parsed.(*SyncTarget)
	require.True(t, ok)
	assert.Equal(t, "file-test", target.Name)
	assert.Equal(t, "lexicore.io/v1", target.APIVersion)
	assert.Equal(t, "SyncTarget", target.Kind)
}

func TestParser_ParseDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	manifests := map[string]string{
		"source.yaml": `
apiVersion: lexicore.io/v1
kind: IdentitySource
metadata:
  name: ldap-source
spec:
  type: ldap
  config:
    url: ldap://localhost
    bindDN: cn=admin,dc=example,dc=com
    bindPassword: secret
    baseDN: ou=users,dc=example,dc=com
`,
		"target.yaml": `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: test-target
spec:
  sourceRef: ldap-source
  operator: mock
`,
		"invalid.txt": "this should be ignored",
	}

	for filename, content := range manifests {
		path := filepath.Join(tmpDir, filename)
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	parser := NewParser()
	parsed, err := parser.ParseDirectory(tmpDir)
	require.NoError(t, err)

	// Should only parse .yaml files
	assert.Len(t, parsed, 2)

	var sources, targets int
	for _, m := range parsed {
		switch obj := m.(type) {
		case *IdentitySource:
			sources++
			assert.Equal(t, "lexicore.io/v1", obj.APIVersion)
		case *SyncTarget:
			targets++
			assert.Equal(t, "lexicore.io/v1", obj.APIVersion)
		}
	}

	assert.Equal(t, 1, sources)
	assert.Equal(t, 1, targets)
}

func TestParser_ParseDirectory_WithInvalidManifest(t *testing.T) {
	tmpDir := t.TempDir()

	manifests := map[string]string{
		"valid.yaml": `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: valid
spec:
  sourceRef: source
  operator: mock
`,
		"invalid.yaml": `
apiVersion: wrongversion
kind: SyncTarget
metadata:
  name: invalid
spec:
  sourceRef: source
  operator: mock
`,
	}

	for filename, content := range manifests {
		path := filepath.Join(tmpDir, filename)
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	parser := NewParser()
	_, err := parser.ParseDirectory(tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported apiVersion")
}
