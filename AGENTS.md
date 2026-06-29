# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository. `CLAUDE.md` is a symlink to this file so that Claude Code auto-loads it; the `AGENTS.md` name is canonical.

HyperShift OADP Plugin is a Velero plugin that enables backup and restore of Hosted Control Planes (HCP) on OpenShift. It registers a Backup Item Action and a Restore Item Action that handle HCP-specific lifecycle operations: etcd snapshot coordination, cloud credential resolution per platform, and ordered control plane recovery.

This file is intentionally minimal — detailed guidance lives in the referenced files below and should be updated there, not here.

## Key References

| Topic                                                          | Where to look                      |
| -------------------------------------------------------------- | ---------------------------------- |
| **Architecture and components**                                | [ARCHITECTURE.md](ARCHITECTURE.md) |
| **Contributing, PR process, CI**                               | [CONTRIBUTING.md](CONTRIBUTING.md) |
| **Build, project layout, quickstart**                          | [README.md](README.md)             |
| **Platform examples (AWS, Azure, Agent, KubeVirt, OpenStack)** | [examples/](examples/)             |

## Type Registration Is Plugin Dispatch

The resource lists in `pkg/core/types/types.go` (`BackupCommonResources`, `BackupAWSResources`, etc.) are **not** a passive inventory of related types. Each entry tells Velero that this plugin must react to that resource kind. When Velero encounters one of these resources during a backup or restore, it invokes the plugin's `Execute()` method, which runs a full reconciliation cycle for that object.

Adding a type to these lists means:

- The plugin's backup action will be called for every instance of that resource in the backup.
- The plugin's restore action will be called for every instance during restore.
- The `Execute()` method in `pkg/core/backup.go` and `pkg/core/restore.go` dispatches on `kind` via switch-case. If you add a new kind, you must add the corresponding case.

Do not add types to these lists unless the plugin needs to take a specific action on them. If a resource just needs to be included in the Velero backup without plugin intervention, it belongs in the Velero `Backup` CR spec, not here.

## Dependency Management

### Minimize dependencies

Keep the dependency footprint small. Before adding a new dependency:

- If you only need one function from a large module, extract and adapt that function instead of importing the whole dependency.
- If the dependency provides substantial value (multiple functions, complex logic, maintained upstream), adding it is fine.
- Evaluate the transitive dependency cost — a small library that pulls in dozens of indirect dependencies is not small.

### Keep dependencies stable and current

- The plugin tracks `github.com/openshift/hypershift/api` as a watched upstream dependency. When HyperShift API types change, this plugin must update to stay compatible.
- Run `make update-deps` to update watched dependencies automatically. Run `make deps` after any `go.mod` change to tidy and vendor.
- Integration tests in `tests/integration/dependencies/` validate that watched dependencies are up to date. CI will fail if they drift.
- Dependency updates should be their own commits, separate from feature work.

## How the Velero Plugin Framework Works

This is not a normal Go executable. The plugin is compiled as a binary but never run directly — Velero discovers and launches it as a gRPC subprocess based on the plugin registrations in `main.go`.

The lifecycle works as follows:

1. **Registration.** At startup, Velero reads the plugin's registered actions (Backup Item Action, Restore Item Action) and the resource types declared in `pkg/core/types/types.go`.
2. **Dispatch.** When Velero processes a Kubernetes object during a backup or restore, it checks if the object's kind matches any registered type. If it does, Velero invokes the plugin's `Execute()` method with that object.
3. **Reconciliation cycle.** Inside `Execute()`, the plugin dispatches on `kind` (switch-case in `pkg/core/backup.go` / `pkg/core/restore.go`) and runs the reconciliation logic for that resource. During this cycle the plugin **can** make changes to cluster state (create CRs, read secrets, update objects), but these side effects must be idempotent — the same cycle will run again when Velero processes the next object of the same kind or any other kind in the registered list.
4. **Return.** After the cycle completes, the plugin returns the object to Velero — potentially modified (annotations added, fields injected, status updated). Velero then backs up or restores that modified version.

Be especially careful with cluster-mutating operations inside `Execute()`. Any action taken (e.g., creating an `HCPEtcdBackup` CR, writing annotations) will execute again for every matching object Velero processes. Design side effects to be safe to repeat.

## Plugin Runtime Context

The plugin runs inside the Velero server pod, not as a standalone binary. It is loaded by Velero's plugin framework via gRPC. This means:

- The plugin shares the Velero pod's service account, RBAC, and network context.
- Cloud credentials are resolved from the environment (AWS STS assume-role, Azure SAS delegation) — the plugin does not manage its own credential lifecycle.
- Logging goes through the Velero logger (`logrus.FieldLogger`), not `fmt.Print` or standalone log setup.

