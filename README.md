# Lexicore

Lexicore is an extensible identity orchestration engine that treats **Identity as Code**. Built on a state-based reconciliation loop, Lexicore synchronizes identities from various sources to downstream service providers, ensuring your infrastructure reflects your central identity provider's state.

By using declarative YAML manifests, Lexicore allows you to define complex selection, mapping, and templating logic to provision accounts across disparate systems like mail servers, Unix systems, and SaaS platforms.

## Key Features

- **Declarative YAML Manifests**: Define `IdentitySource` and `SyncTarget` using a Kubernetes-style syntax.
- **Transformation Pipeline**: 
  - **Selector**: Inclusion/Exclusion based on groups or attributes.
  - **Constant**: Inject static defaults (quotas, domains, shell paths).
  - **Sanitizer**: Perform fine-grained manipulation on individual identity fields (Username, Email, etc.).
  - **Template**: Generate dynamic attributes using Go templating (e.g., `homeDir: "/home/{{.Username}}"`).
- **State-Based Reconciliation**: Calculates diffs (Create/Update/Delete) between source and target to minimize unnecessary operations.
- **Dry-Run Support**: Validate synchronization logic without modifying downstream systems.
- **Environment Variable Expansion**: Inject secrets and dynamic configs into manifests at runtime.
- **Extensible Architecture**: Pluggable Source and Operator drivers.

---

## Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│                     Orchestrator Core                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  Controller  │  │  Reconciler  │  │  Transformer │      │
│  │   Manager    │──│    Loop      │──│   Pipeline   │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│  Source Driver  │  │   Manifest CRD  │  │ Operator Driver │
│     (LDAP)      │  │    Processor    │  │    Registry     │
└─────────────────┘  └─────────────────┘  └─────────────────┘
         │                                          │
         ▼                                          ▼
┌─────────────────┐                        ┌─────────────────┐
│  LDAP (RO)      │                        │   Operators:    │
│  Source of      │                        │  - Dovecot      │
│  Truth          │                        │  - AD           │
│                 │                        │  - PostgreSQL   │
└─────────────────┘                        │  - etc.         │
                                           └─────────────────┘
```

Lexicore operates as a central controller manager:
1. **Source Drivers** (e.g., LDAP) fetch raw identity data.
2. **Transformer Pipelines** process, selector, and enrich the data.
3. **Reconciler** compares the desired state with the current state in the cache.
4. **Operator Drivers** (e.g., Dovecot, Unix, SaaS) execute the necessary changes to reach the desired state.

---

### Configuration Example

#### 1. Identity Source
Define where your users come from:

```yaml
apiVersion: lexicore.io/v1
kind: IdentitySource
metadata:
  name: corporate-ldap
spec:
  type: ldap
  syncPeriod: 5m
  config:
    url: ldap://ldap.example.com
    bindDN: cn=admin,dc=example,dc=com
    bindPassword: ${LDAP_PASSWORD}
    baseDN: ou=users,dc=example,dc=com
```

#### 2. Sync Target
Define how those users should be provisioned into a specific service:

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
    - name: data-cleanup
      type: sanitizer
      config:
        sanitizers:
          - field: email
            type: lowercase
          - field: email
            type: trim
          - field: username
            type: regex
            pattern: "^(.+)@.+$"
            replace: "$1"
    - name: mail-defaults
      type: constant
      config:
        mappings:
          quota: 5242880 # 5GB
          domain: example.com
    - name: path-templates
      type: template
      config:
        templates:
          maildir: "/var/vmail/{{.Attributes.domain}}/{{.Username}}/"
          displayName: "{{.Attributes.firstName}} {{.Attributes.lastName}}"
```

#### Expected Actions & Changes
When this configuration is applied, the orchestrator performs the following changes on the downstream service (**Dovecot**):

*   **Membership Alignment**: Creates new mailboxes for LDAP users in the `developers` group and deletes mailboxes for users who have left that group or LDAP entirely.
*   **Data Sanitization**: Automatically fixes formatting issues by forcing emails to lowercase, trimming whitespace, and stripping domain suffixes from usernames (e.g., `Alice@Example.com` becomes `alice`).
*   **Quota Enforcement**: Forces a strict `5242880` (5GB) limit on all accounts; any manual overrides in the target system will be reverted to this value.
*   **Dynamic Pathing**: Calculates and sets the `maildir` path using the sanitized username and the mapped domain (e.g., `/var/vmail/example.com/alice/`).
*   **Metadata Enrichment**: Updates the target's `displayName` field by merging LDAP `firstName` and `lastName` attributes.
*   **State Drift Correction**: If an account exists but its attributes (like the display name or path) differ from the processed LDAP source, the orchestrator will update only those specific fields.

---

## Transformer Types

Lexicore uses a pipeline architecture where identities are passed through a series of transformers. Each transformer modifies the identity set before it reaches the operator.

### `selector`
Used to include or exclude identities based on specific criteria.
*   **`groupSelector`**: Only includes users who are members of the specified group name.
*   **`emailDomain`**: (Planned) Selectors users based on the domain part of their email address.
*   **Use Case**: Restricting a high-privilege Unix sync target only to the `sysadmins` LDAP group.

### `sanitizer`
Performs fine-grained manipulation on individual identity fields (Username, Email, etc.).
*   **`regex`**: Applies a regular expression pattern and replaces it with a specific value. Useful for stripping suffixes or prefixing IDs.
*   **`lowercase` / `uppercase`**: Standardizes the casing of a field to prevent duplicate accounts in case-sensitive systems.
*   **`trim`**: Removes leading and trailing whitespace from strings.
*   **`compute`**: Generates a new attribute based on a simple expression using other identity fields.

### `constant`
Injects static, hard-coded values into the identity attributes.
*   **`mappings`**: A dictionary of key-value pairs that will be added to every identity. If a key already exists from the source, the constant value will overwrite it.
*   **Use Case**: Setting a global `shell: /bin/bash` or `organization: ACME Corp` for all provisioned accounts.

### `template`
The most powerful transformer, utilizing Go's `text/template` engine for dynamic attribute generation.
*   **`templates`**: A dictionary where the value is a template string. You can access core fields (e.g., `{{.Username}}`) and existing attributes (e.g., `{{.Attributes.department}}`).
*   **Context**: Templates have access to the full Identity object, allowing for complex logic like home directory generation or localized welcome messages.
*   **Use Case**: Generating complex file system paths: `homeDir: "/home/{{.Attributes.region}}/{{.Username}}"`.

---

## Development Status: WIP

> [!WARNING]  
> This repo is currently in heavy development. Expect breaking changes in the API and manifest schema until version `1.0.0`. 

### Roadmap
- [x] Core Reconciliation Loop
- [x] LDAP Source Driver
- [x] Selector, Constant, and Template Transformers
- [ ] First-party operators implementation (AD, Dovecot ACLs, etc.)
- [ ] Webhook source/target drivers
- [ ] Comprehensive Prometheus Metrics

## License

This project is licensed under the MIT License - see the LICENSE file for details.

