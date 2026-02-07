# Plugins (Starlark)

Lexicore supports custom integration logic using the [Starlark](https://bazel.build/rules/language) language (Python-like, embedded). Plugins can act as:

- **Sources**: fetch identities/groups from some system
- **Operators**: apply changes downstream

Both are wired the same way at the manifest level: set the type/operator to `plugin` and point `spec.pluginSource` at a script (file or git).

---

## Plugin loading (`pluginSource`)

`pluginSource` supports two modes:

- `file`: load from a path on disk
- `git`: clone/checkout a repo ref and load a script path

You’ll see both in the full examples below.

---

## Starlark as an IdentitySource (full spec)

When you want a plugin source, set:

- `spec.type: plugin`
- `spec.pluginSource: ...`

```yaml
apiVersion: lexicore.io/v1
kind: IdentitySource
metadata:
  name: hr-system
spec:
  type: plugin
  syncPeriod: 5m
  config:
    api_url: "https://hr.example.com"
    tokenSecretRef: "hr-api-token"
    include_disabled: false

  pluginSource:
    type: git
    git:
      url: https://github.com/myorg/lexicore-plugins.git
      ref: v1.2.0
      path: sources/hr.star
      auth:
        secretRef: git-credentials
```

What your plugin needs to implement:

- `initialize(config)`
- `validate(config)`
- `get_identities(config)`
- `get_groups(config)`
- optional: `connect(config)`
- optional: `close()`

Return shapes are strict-ish: `get_identities` and `get_groups` should return dicts of dicts.

---

## Starlark as a SyncTarget operator (full spec)

When you want a plugin operator, set:

- `spec.operator: plugin`
- `spec.pluginSource: ...`

```yaml
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: mail-provisioning
spec:
  sourceRef: corporate-ldap
  operator: plugin
  dryRun: false

  transformers:
    - name: engineering-only
      type: selector
      config:
        groupSelector: developers

    - name: normalize-username
      type: sanitizer
      config:
        lowercase: true
        trim: true

    - name: templates
      type: template
      config:
        templates:
          maildir: "/var/vmail/{{.Username}}/"
          displayName: "{{.Attributes.firstName}} {{.Attributes.lastName}}"

  config:
    url: "https://mail.example.com/api"
    username: "admin"
    passwordSecretRef: "mail-admin-password"

  pluginSource:
    type: file
    file:
      path: /var/lib/lexicore/plugins/operators/mail.star
```

What your operator plugin needs to implement:

- `initialize(config)`
- `validate(config)`
- `sync(state)`
- optional: `close()`

`sync(state)` gets:

- `state["identities"]` (uid -> identity dict)
- `state["groups"]` (gid -> group dict)
- `state["dry_run"]` (bool)

And it should return counters + error strings.

---

## Error handling

Plugin functions can return:

- `None` for success
- or a dict like `{"error": "..."}`

If your script explodes, Lexicore will surface it as a plugin load/exec failure.

---

## Notes

- `watch` exists on the Go `Source` interface, but plugin sources currently don’t support it (placeholder).
- Git auth is via `git.auth.secretRef`. Keep tokens in Secrets, not in the script.
