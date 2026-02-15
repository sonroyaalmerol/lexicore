# Lexicore

[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/sonroyaalmerol/lexicore)

Lexicore is an extensible identity orchestration engine. Built on a state-based reconciliation loop, Lexicore synchronizes identities from various sources to downstream service providers, ensuring your infrastructure reflects your central identity provider's state.

By using declarative YAML manifests and a Kubernetes-style API, Lexicore allows you to define complex selection, mapping, and templating logic to provision accounts across disparate systems like mail servers, Unix systems, and SaaS platforms.

## Key Features

- **Kubernetes-style API**: Manage resources using `lexictl` or direct REST calls.
- **Embedded HA Storage**: Ships with an integrated etcd server for persistent state and HA without external dependencies.
- **Declarative YAML Manifests**: Define `IdentitySource` and `SyncTarget` resources.
- **Transformation Pipeline**:
  - **Selector**: Inclusion/Exclusion based on groups or attributes.
  - **Sanitizer**: Fine-grained manipulation (Regex, Lowercase, Trim).
  - **Template**: Generate dynamic attributes using Go templating.
- **Worker Pool Architecture**: Scalable reconciliation using configurable worker counts and internal queuing.
- **Starlark-based Plugins**: Extend Lexicore with custom **sources** and **operators** without recompiling the core binary.

---

## Documentation (todo)

- Architecture
- Configuration
- Plugins
- Manifest Spec

---

## Using `lexictl`

### Apply a configuration
```bash
./lexictl apply -f my-resource.yaml
```

### List resources
```bash
./lexictl get synctargets
./lexictl get is  # Short alias for IdentitySources
```

### Delete a resource
```bash
./lexictl delete st email-provisioning
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

### Roadmap
- [x] Embedded HA etcd integration
- [x] Kubernetes-style REST API
- [x] `lexictl` CLI tool
- [x] Core Reconciliation Worker Pool
- [ ] First-party operators (Active Directory, Dovecot ACLs, etc.)
- [ ] First-party sources
- [ ] Third-party operator support
- [ ] Third-party source support
- [ ] Comprehensive Prometheus Metrics
- [ ] Resource Versioning and Conflict Detection
