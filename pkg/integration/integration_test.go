package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/controller"
	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/operator"
	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEndToEndFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	logger, _ := zap.NewDevelopment()

	src := &mockSource{
		identities: []source.Identity{
			{
				UID:      "1",
				Username: "alice",
				Email:    "alice@example.com",
				Groups:   []string{"admins"},
			},
			{
				UID:      "2",
				Username: "bob",
				Email:    "bob@example.com",
				Groups:   []string{"users"},
			},
		},
	}

	op := newMockOperator()

	// Create manifest YAML
	manifestYAML := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: test-sync
spec:
  sourceRef: test-source
  operator: mock
  transformers:
    - name: filter
      type: filter
      config:
        groupFilter: admins
`

	// Parse manifest
	parser := manifest.NewParser()
	parsed, err := parser.Parse([]byte(manifestYAML))
	require.NoError(t, err)

	target, ok := parsed.(*manifest.SyncTarget)
	require.True(t, ok, "Expected SyncTarget manifest")
	require.Equal(t, "test-sync", target.Name)

	reconciler, err := controller.NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = reconciler.Reconcile(ctx)
	require.NoError(t, err)

	assert.True(t, op.syncCalled)
	assert.Equal(t, "Success", target.Status.Status)
	assert.Equal(t, 1, target.Status.IdentityCount) // Only alice (filtered by group)
}

func TestMapTransformer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	logger, _ := zap.NewDevelopment()

	src := &mockSource{
		identities: []source.Identity{
			{
				UID:        "1",
				Username:   "alice",
				Email:      "alice@company.com",
				Attributes: make(map[string]any),
			},
			{
				UID:        "2",
				Username:   "bob",
				Email:      "bob@company.com",
				Attributes: make(map[string]any),
			},
		},
	}

	op := newMockOperator()

	manifestYAML := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: test-map-sync
spec:
  sourceRef: test-source
  operator: mock
  transformers:
    - name: map-defaults
      type: map
      config:
        mappings:
          domain: example.com
          organization: ACME Corp
          quota: 5000
          enabled: true
`

	parser := manifest.NewParser()
	parsed, err := parser.Parse([]byte(manifestYAML))
	require.NoError(t, err)

	target, ok := parsed.(*manifest.SyncTarget)
	require.True(t, ok)

	reconciler, err := controller.NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = reconciler.Reconcile(ctx)
	require.NoError(t, err)

	assert.True(t, op.syncCalled)
	assert.Equal(t, "Success", target.Status.Status)
	assert.Equal(t, 2, target.Status.IdentityCount)
}

func TestTemplateTransformer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	logger, _ := zap.NewDevelopment()

	src := &mockSource{
		identities: []source.Identity{
			{
				UID:      "1",
				Username: "alice",
				Email:    "alice@example.com",
				Attributes: map[string]any{
					"firstName": "Alice",
					"lastName":  "Smith",
				},
			},
			{
				UID:      "2",
				Username: "bob",
				Email:    "bob@example.com",
				Attributes: map[string]any{
					"firstName": "Bob",
					"lastName":  "Johnson",
				},
			},
		},
	}

	op := newMockOperator()

	manifestYAML := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: test-template-sync
spec:
  sourceRef: test-source
  operator: mock
  transformers:
    - name: template-fields
      type: template
      config:
        templates:
          displayName: "{{.Attributes.firstName}} {{.Attributes.lastName}}"
          maildir: "/var/mail/{{.Username}}"
          homeDir: "/home/{{.Username}}"
          fullEmail: "{{.Username}}@example.com"
`

	parser := manifest.NewParser()
	parsed, err := parser.Parse([]byte(manifestYAML))
	require.NoError(t, err)

	target, ok := parsed.(*manifest.SyncTarget)
	require.True(t, ok)

	reconciler, err := controller.NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = reconciler.Reconcile(ctx)
	require.NoError(t, err)

	assert.True(t, op.syncCalled)
	assert.Equal(t, "Success", target.Status.Status)
	assert.Equal(t, 2, target.Status.IdentityCount)
}

func TestFilterTransformer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	logger, _ := zap.NewDevelopment()

	src := &mockSource{
		identities: []source.Identity{
			{
				UID:      "1",
				Username: "alice",
				Email:    "alice@example.com",
				Groups:   []string{"admins", "developers"},
			},
			{
				UID:      "2",
				Username: "bob",
				Email:    "bob@example.com",
				Groups:   []string{"users"},
			},
			{
				UID:      "3",
				Username: "charlie",
				Email:    "charlie@example.com",
				Groups:   []string{"developers", "users"},
			},
			{
				UID:      "4",
				Username: "diana",
				Email:    "diana@example.com",
				Groups:   []string{"guests"},
			},
		},
	}

	op := newMockOperator()

	manifestYAML := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: test-filter-sync
spec:
  sourceRef: test-source
  operator: mock
  transformers:
    - name: filter-developers
      type: filter
      config:
        groupFilter: developers
`

	parser := manifest.NewParser()
	parsed, err := parser.Parse([]byte(manifestYAML))
	require.NoError(t, err)

	target, ok := parsed.(*manifest.SyncTarget)
	require.True(t, ok)

	reconciler, err := controller.NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = reconciler.Reconcile(ctx)
	require.NoError(t, err)

	assert.True(t, op.syncCalled)
	assert.Equal(t, "Success", target.Status.Status)
	assert.Equal(t, 2, target.Status.IdentityCount)
}

func TestCombinedTransformers_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	logger, _ := zap.NewDevelopment()

	src := &mockSource{
		identities: []source.Identity{
			{
				UID:      "1",
				Username: "alice",
				Email:    "alice@company.com",
				Groups:   []string{"admins", "developers"},
				Attributes: map[string]any{
					"department": "Engineering",
				},
			},
			{
				UID:      "2",
				Username: "bob",
				Email:    "bob@company.com",
				Groups:   []string{"users"},
				Attributes: map[string]any{
					"department": "Sales",
				},
			},
			{
				UID:      "3",
				Username: "charlie",
				Email:    "charlie@company.com",
				Groups:   []string{"developers"},
				Attributes: map[string]any{
					"department": "Engineering",
				},
			},
		},
	}

	op := newMockOperator()

	manifestYAML := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: test-combined-sync
spec:
  sourceRef: test-source
  operator: mock
  transformers:
    - name: filter-devs
      type: filter
      config:
        groupFilter: developers
    - name: add-defaults
      type: map
      config:
        mappings:
          domain: example.com
          quota: 10000
          shellAccess: true
          organization: ACME Corp
    - name: compute-fields
      type: template
      config:
        templates:
          homeDir: "/home/{{.Username}}"
          maildir: "/var/mail/{{.Username}}"
          displayName: "{{.Username}} ({{.Attributes.department}})"
          sshKey: "/home/{{.Username}}/.ssh/id_rsa.pub"
`

	parser := manifest.NewParser()
	parsed, err := parser.Parse([]byte(manifestYAML))
	require.NoError(t, err)

	target, ok := parsed.(*manifest.SyncTarget)
	require.True(t, ok)

	reconciler, err := controller.NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = reconciler.Reconcile(ctx)
	require.NoError(t, err)

	assert.True(t, op.syncCalled)
	assert.Equal(t, "Success", target.Status.Status)
	assert.Equal(t, 2, target.Status.IdentityCount)
}

func TestEmailProvisioningWorkflow_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	logger, _ := zap.NewDevelopment()

	src := &mockSource{
		identities: []source.Identity{
			{
				UID:      "1001",
				Username: "jsmith",
				Email:    "john.smith@oldcompany.com",
				Groups:   []string{"employees", "engineering"},
				Attributes: map[string]any{
					"firstName": "John",
					"lastName":  "Smith",
					"title":     "Senior Engineer",
				},
			},
			{
				UID:      "1002",
				Username: "mjones",
				Email:    "mary.jones@oldcompany.com",
				Groups:   []string{"employees", "sales"},
				Attributes: map[string]any{
					"firstName": "Mary",
					"lastName":  "Jones",
					"title":     "Sales Manager",
				},
			},
			{
				UID:      "1003",
				Username: "bwilson",
				Email:    "bob.wilson@oldcompany.com",
				Groups:   []string{"employees", "engineering"},
				Attributes: map[string]any{
					"firstName": "Bob",
					"lastName":  "Wilson",
					"title":     "DevOps Engineer",
				},
			},
		},
	}

	op := newMockOperator()

	manifestYAML := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: email-engineering-sync
spec:
  sourceRef: ldap-source
  operator: dovecot
  transformers:
    - name: engineering-filter
      type: filter
      config:
        groupFilter: engineering
    - name: email-defaults
      type: map
      config:
        mappings:
          domain: newcompany.com
          quota: 5368709120
          enabled: true
          imap: true
          pop3: false
          webmail: true
          spamFilter: true
          virusFilter: true
    - name: email-templates
      type: template
      config:
        templates:
          email: "{{.Username}}@newcompany.com"
          maildir: "/var/vmail/{{.Attributes.domain}}/{{.Username}}/"
          displayName: "{{.Attributes.firstName}} {{.Attributes.lastName}}"
          sieveDir: "/var/vmail/{{.Attributes.domain}}/{{.Username}}/sieve/"
          welcomeMsg: "Welcome {{.Attributes.firstName}}! Your email is {{.Username}}@newcompany.com"
`

	parser := manifest.NewParser()
	parsed, err := parser.Parse([]byte(manifestYAML))
	require.NoError(t, err)

	target, ok := parsed.(*manifest.SyncTarget)
	require.True(t, ok)
	require.Equal(t, "email-engineering-sync", target.Name)
	require.Len(t, target.Spec.Transformers, 3)

	reconciler, err := controller.NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = reconciler.Reconcile(ctx)
	require.NoError(t, err)

	assert.True(t, op.syncCalled)
	assert.Equal(t, "Success", target.Status.Status)
	assert.Equal(t, 2, target.Status.IdentityCount)
}

func TestUnixAccountProvisioning_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	logger, _ := zap.NewDevelopment()

	src := &mockSource{
		identities: []source.Identity{
			{
				UID:      "2001",
				Username: "admin1",
				Email:    "admin1@example.com",
				Groups:   []string{"admins", "sudo"},
				Attributes: map[string]any{
					"employeeId": "E001",
				},
			},
			{
				UID:      "2002",
				Username: "dev1",
				Email:    "dev1@example.com",
				Groups:   []string{"developers"},
				Attributes: map[string]any{
					"employeeId": "E002",
				},
			},
		},
	}

	op := newMockOperator()

	manifestYAML := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: unix-accounts-sync
spec:
  sourceRef: ldap-source
  operator: unix
  transformers:
    - name: unix-defaults
      type: map
      config:
        mappings:
          shell: /bin/bash
          uidNumber: 10000
          gidNumber: 10000
          createHome: true
          passwordHash: "!"
    - name: unix-paths
      type: template
      config:
        templates:
          homeDirectory: "/home/{{.Username}}"
          sshKeyPath: "/home/{{.Username}}/.ssh/authorized_keys"
          bashProfile: "/home/{{.Username}}/.bash_profile"
          gecos: "{{.Username}},{{.Attributes.employeeId}}"
`

	parser := manifest.NewParser()
	parsed, err := parser.Parse([]byte(manifestYAML))
	require.NoError(t, err)

	target, ok := parsed.(*manifest.SyncTarget)
	require.True(t, ok)

	reconciler, err := controller.NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = reconciler.Reconcile(ctx)
	require.NoError(t, err)

	assert.True(t, op.syncCalled)
	assert.Equal(t, "Success", target.Status.Status)
	assert.Equal(t, 2, target.Status.IdentityCount)
}

func TestMultiStageDataEnrichment_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	logger, _ := zap.NewDevelopment()

	src := &mockSource{
		identities: []source.Identity{
			{
				UID:      "3001",
				Username: "alice",
				Email:    "alice@example.com",
				Groups:   []string{"premium-users", "beta-testers"},
				Attributes: map[string]any{
					"tier":       "premium",
					"joinDate":   "2024-01-15",
					"department": "Engineering",
				},
			},
			{
				UID:      "3002",
				Username: "bob",
				Email:    "bob@example.com",
				Groups:   []string{"free-users"},
				Attributes: map[string]any{
					"tier":       "free",
					"joinDate":   "2024-06-20",
					"department": "Sales",
				},
			},
		},
	}

	op := newMockOperator()

	manifestYAML := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: saas-provisioning
spec:
  sourceRef: user-db
  operator: saas-platform
  transformers:
    - name: premium-filter
      type: filter
      config:
        groupFilter: premium-users
    - name: premium-settings
      type: map
      config:
        mappings:
          maxProjects: 100
          maxStorage: 107374182400
          apiRateLimit: 10000
          supportPriority: high
          slaLevel: enterprise
          backupEnabled: true
    - name: resource-generation
      type: template
      config:
        templates:
          apiEndpoint: "https://api.example.com/v1/users/{{.Username}}"
          dashboardUrl: "https://app.example.com/{{.Username}}"
          storageRoot: "/storage/premium/{{.Username}}"
          backupPath: "/backups/premium/{{.Username}}"
          webhookSecret: "{{.Username}}-{{.UID}}-webhook"
          welcomeMessage: "Welcome {{.Username}}! You're on our {{.Attributes.tier}} plan."
`

	parser := manifest.NewParser()
	parsed, err := parser.Parse([]byte(manifestYAML))
	require.NoError(t, err)

	target, ok := parsed.(*manifest.SyncTarget)
	require.True(t, ok)

	reconciler, err := controller.NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = reconciler.Reconcile(ctx)
	require.NoError(t, err)

	assert.True(t, op.syncCalled)
	assert.Equal(t, "Success", target.Status.Status)
	assert.Equal(t, 1, target.Status.IdentityCount)
}

func TestParseFromFile_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	logger, _ := zap.NewDevelopment()

	// Create temporary directory for manifest files
	tmpDir := t.TempDir()

	// Write manifest to file
	manifestYAML := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: file-based-sync
spec:
  sourceRef: test-source
  operator: mock
  transformers:
    - name: add-domain
      type: map
      config:
        mappings:
          domain: example.com
          region: us-west-2
    - name: generate-paths
      type: template
      config:
        templates:
          s3Path: "s3://bucket/{{.Attributes.region}}/{{.Username}}"
`

	manifestPath := filepath.Join(tmpDir, "sync-target.yaml")
	err := os.WriteFile(manifestPath, []byte(manifestYAML), 0644)
	require.NoError(t, err)

	// Parse from file
	parser := manifest.NewParser()
	parsed, err := parser.ParseFile(manifestPath)
	require.NoError(t, err)

	target, ok := parsed.(*manifest.SyncTarget)
	require.True(t, ok)
	require.Equal(t, "file-based-sync", target.Name)
	require.Len(t, target.Spec.Transformers, 2)

	src := &mockSource{
		identities: []source.Identity{
			{
				UID:        "1",
				Username:   "alice",
				Email:      "alice@example.com",
				Attributes: make(map[string]any),
			},
		},
	}

	op := newMockOperator()

	reconciler, err := controller.NewReconciler(target, src, op, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = reconciler.Reconcile(ctx)
	require.NoError(t, err)

	assert.True(t, op.syncCalled)
	assert.Equal(t, "Success", target.Status.Status)
}

func TestParseDirectory_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create temporary directory with multiple manifests
	tmpDir := t.TempDir()

	// Write multiple manifest files
	manifests := map[string]string{
		"source.yaml": `
apiVersion: lexicore.io/v1
kind: IdentitySource
metadata:
  name: ldap-source
spec:
  type: ldap
  syncPeriod: 5m
  config:
    url: ldap://localhost:389
    bindDN: cn=admin,dc=example,dc=com
    bindPassword: secret
    baseDN: ou=users,dc=example,dc=com
`,
		"sync-target-1.yaml": `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: email-sync
spec:
  sourceRef: ldap-source
  operator: dovecot
  transformers:
    - name: add-domain
      type: map
      config:
        mappings:
          domain: mail.example.com
`,
		"sync-target-2.yaml": `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: unix-sync
spec:
  sourceRef: ldap-source
  operator: unix
  transformers:
    - name: unix-defaults
      type: map
      config:
        mappings:
          shell: /bin/bash
`,
	}

	for filename, content := range manifests {
		path := filepath.Join(tmpDir, filename)
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Parse directory
	parser := manifest.NewParser()
	parsed, err := parser.ParseDirectory(tmpDir)
	require.NoError(t, err)

	require.Len(t, parsed, 3)

	// Verify we got the expected types
	var sources, targets int
	for _, m := range parsed {
		switch m.(type) {
		case *manifest.IdentitySource:
			sources++
		case *manifest.SyncTarget:
			targets++
		}
	}

	assert.Equal(t, 1, sources)
	assert.Equal(t, 2, targets)
}

func TestEnvironmentVariableExpansion_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Set environment variables for testing
	os.Setenv("EMAIL_DOMAIN", "dynamic.example.com")
	os.Setenv("QUOTA_SIZE", "8000")
	defer os.Unsetenv("EMAIL_DOMAIN")
	defer os.Unsetenv("QUOTA_SIZE")

	manifestYAML := `
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: env-var-sync
spec:
  sourceRef: test-source
  operator: mock
  transformers:
    - name: env-based-config
      type: map
      config:
        mappings:
          domain: ${EMAIL_DOMAIN}
          quota: ${QUOTA_SIZE}
`

	parser := manifest.NewParser()
	parsed, err := parser.Parse([]byte(manifestYAML))
	require.NoError(t, err)

	target, ok := parsed.(*manifest.SyncTarget)
	require.True(t, ok)
	require.Len(t, target.Spec.Transformers, 1)

	// Verify environment variables were expanded
	mappings := target.Spec.Transformers[0].Config["mappings"].(map[string]any)
	assert.Equal(t, "dynamic.example.com", mappings["domain"])
	assert.Equal(t, 8000, int(mappings["quota"].(uint64)))
}

// Mock implementations
type mockSource struct {
	identities []source.Identity
	groups     []source.Group
}

func (m *mockSource) Connect(ctx context.Context) error { return nil }
func (m *mockSource) GetIdentities(ctx context.Context) ([]source.Identity, error) {
	return m.identities, nil
}
func (m *mockSource) GetGroups(ctx context.Context) ([]source.Group, error) {
	return m.groups, nil
}
func (m *mockSource) Watch(ctx context.Context) (<-chan source.Event, error) {
	return make(chan source.Event), nil
}
func (m *mockSource) Close() error { return nil }

type mockOperator struct {
	*operator.BaseOperator
	syncCalled bool
	syncResult *operator.SyncResult
}

func newMockOperator() *mockOperator {
	return &mockOperator{
		BaseOperator: operator.NewBaseOperator("mock"),
		syncResult:   &operator.SyncResult{IdentitiesCreated: 1},
	}
}

func (m *mockOperator) Initialize(
	ctx context.Context,
	config map[string]any,
) error {
	m.SetConfig(config)
	return nil
}

func (m *mockOperator) Sync(
	ctx context.Context,
	state *operator.SyncState,
) (*operator.SyncResult, error) {
	m.syncCalled = true
	m.syncResult.IdentitiesCreated = len(state.Identities)
	return m.syncResult, nil
}

func (m *mockOperator) Validate(ctx context.Context) error { return nil }
func (m *mockOperator) Close() error                       { return nil }
