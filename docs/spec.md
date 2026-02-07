# Manifest Spec

This doc describes the current manifest shapes used by Lexicore resources.

---

## IdentitySource

### Shape

```yaml
apiVersion: lexicore.io/v1
kind: IdentitySource
metadata:
  name: example
spec:
  type: ldap|okta|starlark|...
  config: {}          # provider-specific blob
  syncPeriod: 5m      # string duration
  pluginSource: {}    # optional (only for type: starlark)
```

### Fields

- `spec.type` (string)
  - Source driver type (`ldap`, `okta`, `starlark`, etc.)
- `spec.config` (map)
  - Driver-specific config passed into the source implementation
- `spec.syncPeriod` (string)
  - How often this source should refresh (duration string)
- `spec.pluginSource` (object, optional)
  - Where to fetch the Starlark script (only meaningful for `type: starlark`)

### Example (starlark + git)

```yaml
apiVersion: lexicore.io/v1
kind: IdentitySource
metadata:
  name: hr-system
spec:
  type: starlark
  syncPeriod: 10m
  config:
    api_url: "https://hr.example.com"
    tokenSecretRef: "hr-api-token"
  pluginSource:
    type: git
    git:
      url: https://github.com/myorg/lexicore-plugins.git
      ref: v1.2.0
      path: sources/hr.star
      auth:
        secretRef: git-credentials
```

---

## SyncTarget

### Shape

```yaml
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: example
spec:
  sourceRef: some-identity-source
  operator: dovecot|unix|starlark|...
  transformers: []
  config: {}
  dryRun: false
  pluginSource: {} # optional (only for operator: starlark)
status: {}         # controller-populated
```

### Fields

- `spec.sourceRef` (string)
  - Name of the `IdentitySource` this target consumes
- `spec.operator` (string)
  - Operator driver (`dovecot`, `unix`, `starlark`, etc.)
- `spec.transformers` (array)
  - Transformation pipeline, evaluated in order
- `spec.config` (map)
  - Operator-specific config passed into the operator implementation
- `spec.dryRun` (bool)
  - If true, reconciliation should compute actions but not apply them
- `spec.pluginSource` (object, optional)
  - Where to fetch the Starlark operator plugin (only meaningful for `operator: starlark`)

### Example (starlark + file)

```yaml
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: mail-provisioning
spec:
  sourceRef: corporate-ldap
  operator: starlark
  dryRun: true
  transformers:
    - name: engineering-only
      type: selector
      config:
        groupSelector: developers
  config:
    url: "https://mail.example.com/api"
    username: "admin"
    passwordSecretRef: "mail-admin-password"
  pluginSource:
    type: file
    file:
      path: /var/lib/lexicore/plugins/operators/mail.star
```

---

## Transformers

Each transformer is:

```yaml
- name: some-step
  type: selector|sanitizer|template|...
  config: {}
```

Fields:

- `name` (string): label for humans/logs
- `type` (string): transformer type
- `config` (map): transformer-specific config blob

---

## PluginSource

Used by both `IdentitySource.spec.pluginSource` and `SyncTarget.spec.pluginSource`.

### Shape

```yaml
pluginSource:
  type: file|git
  file:
    path: /path/to/script.star
  git:
    url: https://github.com/org/repo.git
    ref: v1.2.3
    path: operators/mail.star
    auth:
      secretRef: git-credentials
```

### `file`

- `pluginSource.type: file`
- `pluginSource.file.path` (string): path to the `.star` script

### `git`

- `pluginSource.type: git`
- `pluginSource.git.url` (string): repo URL
- `pluginSource.git.ref` (string, optional): branch/tag/SHA
- `pluginSource.git.path` (string): path in repo
- `pluginSource.git.auth.secretRef` (string, optional): secret reference for auth

---

## SyncTarget Status

`status` is owned by the controller.

### Shape

```yaml
status:
  lastSync: "2024-01-15T10:30:00Z"
  status: "Synced|Error|..."
  message: "..."
  identityCount: 142
  groupCount: 12
  pluginStatus:
    loaded: true
    gitCommit: "a1b2c3d4"
    lastUpdated: "2024-01-15T09:00:00Z"
    error: ""
```

### `pluginStatus`

- `loaded` (bool): plugin load success
- `gitCommit` (string, optional): resolved commit when git-based
- `lastUpdated` (timestamp, optional): last fetch/update time
- `error` (string, optional): load/exec error message
