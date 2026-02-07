# Configuration

Lexicore uses a structured `config.yaml` file loaded at startup via the `--config` flag.

It also supports environment variable overrides via `envconfig` (see the struct tags in `pkg/config`).

## Full Reference (`config.yaml`)

```yaml
server:
  address: ":8080"            # REST API bind address
  healthCheck: true           # Enables /healthz
  metrics: true               # Enables /metrics on the API server (if implemented)
  pluginsDir: "/var/lib/lexicore/plugins"

logging:
  level: "info"               # debug, info, warn, error
  format: "json"              # console, json
  output: "stdout"            # stdout/stderr or file path

metrics:
  enabled: true
  port: 9090
  path: "/metrics"

etcd:
  # If endpoints is non-empty, Lexicore connects to an external etcd cluster.
  # If endpoints is empty [], Lexicore runs embedded etcd.
  endpoints: []

  # Embedded etcd settings
  dataDir: "/var/lib/lexicore/etcd"

  # Cluster formation
  autoJoin: false             # If true, join via discovery/seed nodes
  discovery: "static"         # "static" or a discovery backend (impl-dependent)

  # Static/manual config (used when autoJoin=false OR discovery="static")
  name: "node-1"
  peerAddr: "http://localhost:2380"
  clientAddr: "http://localhost:2379"
  initialCluster: "node-1=http://localhost:2380"

  # Join helpers / network
  bindAddr: "0.0.0.0"
  seedAddrs: []

workers:
  reconcileWorkers: 4
  queueSize: 100

defaultSyncPeriod: 5m
```

## Environment variable overrides (examples)

Lexicore uses `envconfig` to override values from the environment after loading defaults + `config.yaml`.

A few practical examples:

```bash
# Global
export DEFAULT_SYNC_PERIOD="2m"

# Server
export ADDRESS=":8081"
export HEALTH_CHECK="true"
export METRICS="true"
export PLUGINS_DIR="/var/lib/lexicore/plugins"

# Logging
export LEVEL="debug"
export FORMAT="json"
export OUTPUT="stdout"

# Metrics exporter
export ENABLED="true"
export PORT="9091"
export PATH="/metrics"

# etcd
export ENDPOINTS="http://etcd-1:2379,http://etcd-2:2379,http://etcd-3:2379"
export DATA_DIR="/var/lib/lexicore/etcd"
export AUTO_JOIN="true"
export DISCOVERY="static"

export NAME="node-2"
export PEER_ADDR="http://0.0.0.0:2380"
export CLIENT_ADDR="http://0.0.0.0:2379"
export INITIAL_CLUSTER="node-1=http://10.0.0.11:2380,node-2=http://10.0.0.12:2380,node-3=http://10.0.0.13:2380"

export BIND_ADDR="0.0.0.0"
export SEED_ADDRS="10.0.0.11,10.0.0.12"

# Workers
export RECONCILE_WORKERS="8"
export QUEUE_SIZE="500"
```

### Notes

- `envconfig` tags in the config structs define the env var names (e.g. `DEFAULT_SYNC_PERIOD`, `PLUGINS_DIR`).
- For list fields like `ENDPOINTS` and `SEED_ADDRS`, values are typically comma-separated.
- Be careful with generic names like `PORT`, `PATH`, `ENABLED`, `LEVEL`. They’re convenient locally, but they can collide with existing environment variables in containers/CI.
  If this becomes painful, the usual fix is switching to a prefix (e.g. `LEXICORE_PORT`) and updating the `envconfig.Process()` call accordingly.

## Notes

### Embedded vs External etcd
- `etcd.endpoints: []` means **embedded** etcd.
- Setting `etcd.endpoints` to one or more URLs switches Lexicore to **external mode**.

### Plugin directory
`server.pluginsDir` is where Lexicore can look for Starlark plugin scripts (depending on how you load plugins in manifests).

### Environment variables
Some fields can be overridden by env vars (for example `DEFAULT_SYNC_PERIOD` for `defaultSyncPeriod`).
The exact env var names are defined by the `envconfig` tags in the config structs.
