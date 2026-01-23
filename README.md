# Lexicore

Lexicore is an extensible identity orchestration engine that treats **Identity as Code**. Built on a state-based reconciliation loop, Lexicore synchronizes identities from various sources to downstream service providers, ensuring your infrastructure reflects your central identity provider's state.

By using declarative YAML manifests and a Kubernetes-style API, Lexicore allows you to define complex selection, mapping, and templating logic to provision accounts across disparate systems like mail servers, Unix systems, and SaaS platforms.

## Key Features

- **Kubernetes-style API**: Manage resources using `lexictl` or direct REST calls.
- **Embedded HA Storage**: Ships with an integrated etcd server for persistent state and high availability without external dependencies.
- **Declarative YAML Manifests**: Define `IdentitySource` and `SyncTarget` resources.
- **Transformation Pipeline**: 
  - **Selector**: Inclusion/Exclusion based on groups or attributes.
  - **Sanitizer**: Fine-grained manipulation (Regex, Lowercase, Trim).
  - **Template**: Generate dynamic attributes using Go templating.
- **Worker Pool Architecture**: Scalable reconciliation using configurable worker counts and internal queuing.

---

## Architecture

Lexicore operates as a distributed controller manager:
1. **API Server**: Receives manifests and persists them to the **etcd Store**.
2. **Controller Manager**: Watches the store and hydrates a worker pool.
3. **Source Drivers**: Fetch raw identity data from providers (e.g., LDAP).
4. **Transformer Pipeline**: Processes, filters, and enriches data in memory.
5. **Operator Drivers**: Execute changes on downstream services (e.g., Dovecot, Unix).

---

## System Configuration

Lexicore uses a structured `config.yaml` file to manage its runtime environment. This file is loaded at startup via the `--config` flag.

```yaml
# API Server settings
server:
  address: ":8080"           # The host:port the REST API will bind to
  healthCheck: true         # Enables the /healthz endpoint
  metrics: true              # Enables the /metrics endpoint (Prometheus)

# Observability and Logs
logging:
  level: "info"              # Options: debug, info, warn, error
  format: "console"          # Options: console (human-readable), json (structured)
  output: "stdout"           # Path to log file or "stdout"/"stderr"

# Metrics Exporter
metrics:
  enabled: true
  port: 9090                 # Port for the dedicated metrics server
  path: "/metrics"           # Path for Prometheus scraping

# Persistence Layer (etcd)
etcd:
  # Endpoints: If provided, Lexicore connects to an external cluster.
  # If empty [], Lexicore starts an internal Embedded HA instance.
  endpoints: []              
  dataDir: "lexicore.etcd"   # Storage path for embedded data
  name: "node-1"             # Unique name for this node in the cluster
  clientAddr: "http://localhost:2379" # API communication address
  peerAddr: "http://localhost:2380"   # Node-to-node Raft communication
  # Definition of all nodes in the HA cluster
  initialCluster: "node-1=http://localhost:2380"

# Performance and Concurrency
workers:
  reconcileWorkers: 4        # Concurrent reconciliation threads
  queueSize: 100             # Internal buffer for pending sync tasks

defaultSyncPeriod: 5m        # Interval between scheduled sync cycles
```

### Configuration Details

| Section | Key | Description |
| :--- | :--- | :--- |
| **server** | `address` | Defines the entry point for `lexictl` and external integrations. |
| **logging** | `level` | Set to `debug` for detailed reconciliation logs, including diff calculations. |
| **etcd** | `endpoints` | Providing a list of IPs here disables the internal etcd and switches to "External Mode." |
| **etcd** | `initialCluster` | To form an HA cluster, list all members here: `n1=http://...:2380,n2=http://...:2380`. |
| **workers** | `reconcileWorkers` | Controls how many SyncTargets can be processed simultaneously. Increase this if you have hundreds of targets. |
| **workers** | `queueSize` | Prevents system overload by buffering requests during high-frequency changes. |
| `defaultSyncPeriod` | - | The "heartbeat" of the system. Even if no changes are detected via API, the system re-verifies all targets at this interval. |

---

## Deployment Modes

### 1. Single-Node (Embedded)
Ideal for small environments or localized identity management.
- Set `etcd.endpoints` to `[]`.
- Run a single instance of `lexicore`.

### 2. High Availability (Embedded Cluster)
Lexicore nodes form their own Raft cluster. No external database required.
- Set `etcd.name` uniquely on each node.
- Set `etcd.initialCluster` to include all node peer addresses.
- Load-balance the `server.address` across all nodes.

### 3. External (K8s / Cloud Style)
Offload state management to a dedicated etcd cluster (like a production K8s etcd).
- Provide the cluster IPs in `etcd.endpoints`.
- Internal etcd server will be automatically disabled.

---

## Using `lexictl`

Lexicore includes a command-line tool, `lexictl`, to manage resources similarly to `kubectl`.

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
Define where your users come from:

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
Define where those users should be provisioned:

```yaml
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: email-provisioning
spec:
  sourceRef: corporate-ldap
  operator: dovecot
  transformers:
    - name: engineering-only
      type: selector
      config:
        groupSelector: developers
    - name: path-templates
      type: template
      config:
        templates:
          maildir: "/var/vmail/{{.Username}}/"
          displayName: "{{.Attributes.firstName}} {{.Attributes.lastName}}"
```

---

## Transformer Types

### `selector`
Filters identities before they reach the operator.
*   **`groupSelector`**: Only includes users who are members of a specific group.
*   **Use Case**: Restricting Unix shell access to the `sysadmins` group.

### `sanitizer`
Standardizes identity fields.
*   **`regex`**: Regex replacement for field cleanup.
*   **`lowercase` / `uppercase`**: Enforces casing standards.
*   **`trim`**: Removes accidental whitespace.

### `template`
Dynamic attribute generation using Go's `text/template`.
*   **Context**: Access to `{{.Username}}`, `{{.Email}}`, and `{{.Attributes}}`.
*   **Use Case**: `homeDir: "/home/{{.Attributes.department}}/{{.Username}}"`.

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
- [ ] Comprehensive Prometheus Metrics
- [ ] Resource Versioning and Conflict Detection

## License

This project is licensed under the MIT License - see the LICENSE file for details.
