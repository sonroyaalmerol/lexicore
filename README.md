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
- [ ] First-party sources
- [ ] Third-party operator support
- [ ] Third-party source support
- [ ] Comprehensive Prometheus Metrics
- [ ] Resource Versioning and Conflict Detection

## Plugin Development and Usage

Lexicore supports **custom operator drivers** written in [**Starlark**](https://bazel.build/rules/language), a Python-like embedded scripting language. This allows you to extend Lexicore to integrate with any system without recompiling the core binary.

---

### Overview

Starlark plugins enable you to:
- **Provision identities** to custom databases, APIs, or legacy systems
- **Use rich built-in libraries** (HTTP, SQL, LDAP, JSON, hashing, templating)
- **Version control** your integration logic alongside manifests
- **Hot-reload** changes without restarting Lexicore

Plugins are referenced in `SyncTarget` manifests via the `pluginSource` field.

---

### Plugin Architecture

A Starlark operator plugin must implement **three required functions**:

| Function | Purpose | Input | Output |
|:---------|:--------|:------|:-------|
| `initialize(config)` | Setup connections, validate config | `dict` of SyncTarget `config` | `None` or `{"error": "..."}` |
| `validate(config)` | Pre-flight checks | `dict` of SyncTarget `config` | `None` or `{"error": "..."}` |
| `sync(state)` | Reconcile identities | `dict` with `identities`, `groups`, `dry_run` | `dict` with counters and errors |

#### Optional Functions
- `close()` – Cleanup resources (DB connections, HTTP clients)

---

### Available Libraries

Lexicore provides a comprehensive standard library for Starlark plugins:

#### HTTP Client
```python
# Simple requests
resp = http.get("https://api.example.com/users")
resp = http.post(url, data=json.encode(payload), headers={"Content-Type": "application/json"})

# Custom client with options
client = http.client(timeout=60, insecure_skip_verify=True)
```

#### SQL Databases
Supports **PostgreSQL**, **MySQL**, **SQLite**, and **MSSQL**:

```python
# PostgreSQL
db = sql.postgres(
    host="localhost",
    port=5432,
    user="admin",
    password="secret",
    dbname="identity",
    sslmode="require",
    max_open_conns=10
)

# SQLite
db = sql.sqlite(path="/var/lib/data.db")

# Execute queries
db.exec("INSERT INTO users (email, name) VALUES ($1, $2)", [email, name])
rows = db.query("SELECT * FROM users WHERE disabled = $1", [False])
user = db.query_row("SELECT * FROM users WHERE email = $1", [email])

# Transactions
tx = db.begin()
tx.exec("UPDATE users SET active = TRUE WHERE id = $1", [user_id])
tx.commit()  # or tx.rollback()
```

#### LDAP
```python
conn = ldap.connect("ldaps://ldap.corp.com:636", use_tls=True)
conn.bind("cn=admin,dc=corp,dc=com", "password")
results = conn.search("ou=users,dc=corp,dc=com", filter="(objectClass=person)")
conn.close()
```

#### JSON
```python
data = {"key": "value", "nested": {"count": 42}}
encoded = json.encode(data, indent=2)
decoded = json.decode('{"status": "ok"}')
```

#### Base64
```python
encoded = base64.encode("Hello, World!")
decoded = base64.decode("SGVsbG8sIFdvcmxkIQ==")
```

#### Time
```python
now = time.now()                          # RFC3339 string
timestamp = time.unix()                   # Unix seconds
parsed = time.parse("2006-01-02", "2024-01-15")
formatted = time.format(timestamp, "2006-01-02 15:04:05")
```

#### Hashing
```python
hash.md5("data")
hash.sha1("data")
hash.sha256("data")
hash.sha512("data")
```

---

### Plugin Loading Methods

#### 1. File-Based (Local Development)

```yaml
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: database-sync
spec:
  sourceRef: corporate-ldap
  operator: starlark
  pluginSource:
    type: file
    file:
      path: /etc/lexicore/plugins/database.star
  config:
    db_type: postgres
    host: localhost
    user: lexicore
    password: secret
    database: identity
```

**Use Case**: Quick iteration during plugin development.

---

#### 2. Git-Based (Production)

```yaml
apiVersion: lexicore.io/v1
kind: SyncTarget
metadata:
  name: mail-provisioning
spec:
  sourceRef: corporate-ldap
  operator: starlark
  pluginSource:
    type: git
    git:
      url: https://github.com/myorg/lexicore-plugins.git
      ref: v1.2.0  # Tag, branch, or commit SHA
      path: operators/mail.star
      auth:
        secretRef: git-credentials  # Optional for private repos
  config:
    url: "https://mail.example.com/api"
    username: "admin"
    password: ""
```

**Benefits**:
- Version-controlled plugins
- Atomic updates via Git tags
- Multi-environment deployments (dev/staging/prod)

**Git Authentication Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: git-credentials
type: Opaque
stringData:
  username: github-user
  token: ghp_xxxxxxxxxxxx
```

---

### Example Plugins

#### 1. Database Sync Plugin

```python
# database.star
name = "database"

def initialize(config):
    db_type = config.get("db_type", "postgres")
    global _db
    
    if db_type == "postgres":
        _db = sql.postgres(
            host=config["host"],
            user=config["user"],
            password=config["password"],
            dbname=config["database"],
            sslmode=config.get("sslmode", "require")
        )
    
    _db.ping()
    return None

def validate(config):
    required = ["db_type", "host", "user", "password", "database"]
    for field in required:
        if not config.get(field):
            return {"error": "%s is required" % field}
    return None

def sync(state):
    result = {
        "identities_created": 0,
        "identities_updated": 0,
        "identities_deleted": 0,
        "errors": []
    }
    
    tx = _db.begin()
    
    for uid, identity in state["identities"].items():
        email = identity["email"]
        if identity["disabled"]:
            continue
        
        existing = _db.query_row(
            "SELECT id FROM users WHERE email = $1", 
            [email]
        )
        
        if not existing:
            if not state["dry_run"]:
                tx.exec(
                    "INSERT INTO users (email, username, name) VALUES ($1, $2, $3)",
                    [email, identity["username"], identity["display_name"]]
                )
            result["identities_created"] += 1
        else:
            if not state["dry_run"]:
                tx.exec(
                    "UPDATE users SET name = $1 WHERE email = $2",
                    [identity["display_name"], email]
                )
            result["identities_updated"] += 1
    
    tx.commit()
    return result

def close():
    _db.close()
```

---

#### 2. HTTP API Plugin

```python
# mail.star
name = "mail"

def initialize(config):
    global _client, _base_url, _token
    
    _base_url = config["url"]
    _client = http.client(timeout=60)
    
    # Authenticate
    resp = http.post(
        _base_url + "/api/login",
        data=json.encode({
            "username": config["username"],
            "password": config["password"]
        }),
        headers={"Content-Type": "application/json"}
    )
    
    if not resp["ok"]:
        return {"error": "Authentication failed"}
    
    _token = json.decode(resp["body"])["token"]
    return None

def validate(config):
    required = ["url", "username", "password"]
    for field in required:
        if not config.get(field):
            return {"error": "%s is required" % field}
    return None

def sync(state):
    result = {
        "identities_created": 0,
        "identities_updated": 0,
        "errors": []
    }
    
    headers = {
        "Authorization": "Bearer " + _token,
        "Content-Type": "application/json"
    }
    
    for uid, identity in state["identities"].items():
        email = identity["email"]
        
        # Check if user exists
        resp = http.get(
            _base_url + "/api/users/" + email,
            headers=headers
        )
        
        if resp["status_code"] == 404:
            # Create user
            if not state["dry_run"]:
                payload = {
                    "email": email,
                    "name": identity["display_name"],
                    "quota": identity["attributes"].get("mailQuota", "5G")
                }
                create_resp = http.post(
                    _base_url + "/api/users",
                    data=json.encode(payload),
                    headers=headers
                )
                if not create_resp["ok"]:
                    result["errors"].append("Failed to create " + email)
                    continue
            result["identities_created"] += 1
        elif resp["ok"]:
            # Update user
            result["identities_updated"] += 1
    
    return result
```

---

### Plugin Status Tracking

Lexicore tracks plugin metadata in the `SyncTarget` status:

```yaml
status:
  lastSync: "2024-01-15T10:30:00Z"
  status: "Synced"
  identityCount: 142
  pluginStatus:
    loaded: true
    gitCommit: "a1b2c3d4"
    lastUpdated: "2024-01-15T09:00:00Z"
```

---

### Best Practices

#### 1. **Error Handling**
Always return structured errors:
```python
if not config.get("required_field"):
    return {"error": "required_field is missing"}
```

#### 2. **Dry Run Support**
Respect the `state["dry_run"]` flag:
```python
if state["dry_run"]:
    print("[DRY RUN] Would create user:", email)
else:
    db.exec("INSERT INTO users ...")
```

#### 3. **Transaction Safety**
Use database transactions for atomicity:
```python
tx = db.begin()
try:
    # ... operations ...
    tx.commit()
except Exception as e:
    tx.rollback()
    result["errors"].append(str(e))
```

#### 4. **Resource Cleanup**
Implement `close()` to avoid connection leaks:
```python
def close():
    _db.close()
    _http_client = None
```

#### 5. **Logging**
Use `print()` for observability (appears in Lexicore logs):
```python
print("Synced", result["identities_created"], "users to database")
```

### Troubleshooting

#### Plugin fails to load
Check `SyncTarget` status:
```bash
lexictl get st mail-provisioning -o yaml
```

Look for:
```yaml
status:
  pluginStatus:
    loaded: false
    error: "failed to execute starlark script: ..."
```

#### Missing required functions
Error:
```
starlark script missing required function: sync
```

**Solution**: Ensure your plugin defines `initialize`, `validate`, and `sync`.

#### Database connection errors
Enable debug logging in `config.yaml`:
```yaml
logging:
  level: debug
```

Check logs for SQL errors or connection timeouts.

#### Git authentication failures
Verify the secret exists and contains valid credentials:
```bash
lexictl get secret git-credentials -o yaml
```

Ensure the token has repository read permissions.

---

### Security Considerations

1. **Secrets Management**: Never hardcode credentials in plugins. Use `config` parameters and Kubernetes Secrets.

2. **Sandboxing**: Starlark runs in a sandboxed environment. It cannot:
   - Access arbitrary files
   - Execute shell commands
   - Import external Python modules

3. **Resource Limits**: Set timeouts for HTTP/SQL operations:
   ```python
   client = http.client(timeout=30)
   ```

4. **Input Validation**: Always validate `config` in the `validate()` function.

