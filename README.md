# Lexicore (WIP)

Lexicore is an extensible identity orchestration engine that treats "Identity as Code." By implementing a state-based reconciliation loop, it ensures that your downstream service providers (LDAP, AD, Mail, etc.) perfectly reflect the security posture and access rules defined in your central identity provider.

> [!WARNING]  
> This repo is currently in heavy development. Expect major changes on every release until the first stable release, `1.0.0`.
> Do not expect it to work perfectly (or at all) in your specific setup as I have yet to build any tests for this project yet.
> However, feel free to post issues if you think it will be helpful for the development of this project.

## High-Level Architecture

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

## Key Features

1. **Pluggable Sources**: Easy to add new identity sources beyond LDAP
2. **Operator Pattern**: Each target service gets its own operator
3. **Transformation Pipeline**: Flexible data transformation between source and target
4. **Declarative Manifests**: Kubernetes-style YAML configuration
5. **Dry-run Mode**: Test before applying changes
6. **Reconciliation Loop**: Continuous sync with configurable intervals
7. **State Management**: Diff calculation to minimize changes
8. **Error Handling**: Graceful error handling with logging
9. **Extensible**: Easy to add new operators and transformers

