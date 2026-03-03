# Lexicore

Lexicore is an extensible identity ACLs orchestration engine. Built on a state-based reconciliation loop, Lexicore synchronizes identities from various sources to downstream service providers, ensuring your infrastructure reflects your central identity provider's state.

By using declarative YAML manifests and a Kubernetes-style API, Lexicore allows you to define complex selection, mapping, and templating logic to provision accounts across disparate systems like mail servers, Unix systems, and SaaS platforms.

## Key Features

- **Kubernetes-style API**: Manage resources using `lexictl` or direct REST calls.
- **Declarative YAML Manifests**: Define `IdentitySource` and `SyncTarget` resources.
- **Transformation Pipeline**:
  - **Selector**: Inclusion/Exclusion based on groups or attributes.
  - **Sanitizer**: Simple string manipulation functions (Regex, Lowercase, Trim).
  - **Template**: Generate dynamic attributes using Go templating. Supports expansion of arrays (see examples)
- **Lua-based Plugins** (not yet ready): Extend Lexicore with custom **sources** and **operators** without recompiling the core binary.

---

## Documentation (todo)

- Architecture
- Configuration
- Plugins
- Manifest Spec

---

## Using `lexictl`

### List resources
```bash
./lexictl get synctargets
./lexictl get is  # Short alias for IdentitySources
```

---

## Resource Examples

### 1. Identity Source (`IdentitySource`)

```yaml
apiVersion: lexicore.io/v1
kind: IdentitySource
metadata:
  name: corporate-ldap
spec:
  type: ldap
  config:
    url: ldap://ldap.example.com
    baseDN: ou=users,dc=example,dc=com
```

### 2. Sync Target (`SyncTarget`)

```yaml
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: email-provisioning
spec:
  sourceRef: corporate-ldap
  operator: dovecot-acl
  transformers:
    - name: path-templates
      type: template
      config:
        templates:
          acl: "test@sgl.com/INBOX,lookup read write-seen write-deleted"
```

---

## Development Status: WIP

> [!WARNING]
> Lexicore is currently in heavy development. Expect breaking changes in the API and manifest schema until version `1.0.0`.
