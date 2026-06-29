# HyperShift OADP Plugin

A [Velero](https://velero.io/) / [OADP](https://docs.openshift.com/container-platform/latest/backup_and_restore/application_backup_and_restore/oadp-features-plugins.html) plugin that enables backup and restore of Hosted Control Planes (HCP) on OpenShift. It registers two Velero plugin actions — a Backup Item Action and a Restore Item Action — that handle the HCP-specific lifecycle operations required to safely snapshot and recover a HostedCluster and its control plane components.

## Why This Exists

Standard Velero backup/restore does not understand HCP semantics. Backing up a HostedCluster involves more than persisting Kubernetes resources: etcd snapshots must be coordinated, cloud credentials resolved per platform (AWS STS, Azure SAS, Agent), and control plane resources restored in the correct order with updated references. This plugin encodes that domain knowledge so OADP can treat a HostedCluster as a first-class backup target.

## Supported Platforms

- **AWS** (including STS credential resolution and S3 pre-signed URLs for etcd snapshots)
- **Azure** (including Blob SAS URL signing for etcd snapshots)
- **BareMetal / Agent**
- **KubeVirt**
- **OpenStack**

## Quickstart

1. Deploy an OCP management cluster with HyperShift running.
2. Deploy OADP using the sample subscription in [`examples/`](./examples/).
3. Create cloud credentials for your storage provider:
   ```bash
   kubectl create secret generic cloud-credentials \
       --namespace openshift-adp \
       --from-file cloud=<CLOUD_CREDS_FILE>
   ```
4. Create the `DataProtectionApplication` CR (samples in [`examples/`](./examples/)).
5. Create and apply a `Backup` CR (samples in [`examples/`](./examples/)).
6. Check the backup status.

## Building and Running Locally

**Prerequisites:** Go 1.25+, make, Docker/OrbStack (for container builds).

```bash
# Run tests and build the binary
make verify
make local

# Build the container image
make docker-build IMG=<your-registry>/hypershift-oadp-plugin:dev
```

Key `make` targets:

| Target | Description |
|--------|-------------|
| `make local` | Build the plugin binary to `dist/` |
| `make test` | Run all unit and integration tests |
| `make cover` | Run tests with coverage |
| `make verify` | Run module verification + tests |
| `make docker-build` | Build the container image |
| `make deps` | Tidy and vendor Go modules |
| `make update-deps` | Update watched upstream dependencies |
| `make clean` | Remove build artifacts |

## Project Layout

| Component | Directory | Role |
|-----------|-----------|------|
| **Plugin Entry Point** | `main.go` | Velero plugin server — registers the Backup Item Action and Restore Item Action. |
| **Core Plugin Logic** | `pkg/core/` | Backup and Restore plugin implementations, shared types (`core/types/`), and backup validation (`core/validation/`). |
| **Common Utilities** | `pkg/common/` | Shared utilities: credential resolution, scheme registration, helper functions. |
| **AWS Platform** | `pkg/platform/aws/` | AWS-specific platform logic for backup and restore. |
| **Agent Platform** | `pkg/platform/agent/` | Agent (BareMetal) platform logic. |
| **S3 Pre-signed URLs** | `pkg/s3presign/` | AWS S3 pre-signed URL generation and STS credential support for etcd snapshots. |
| **Azure Blob SAS** | `pkg/azblobsas/` | Azure Blob SAS token generation and delegation for etcd snapshots. |
| **Etcd Backup Orchestrator** | `pkg/etcdbackup/` | Etcd backup CR orchestration — coordinates snapshot creation and upload. |
| **Version** | `pkg/version/` | Build version metadata. |
| **Documentation** | `docs/` | Technical reference documentation (DataMover, HCPEtcdBackup). |
| **Examples** | `examples/` | Platform-specific OADP CR samples (AWS, BareMetal, KubeVirt, OpenStack). |
| **Integration Tests** | `tests/integration/` | Dependency validation, S3 pre-sign, and Renovate config tests. |

## Documentation

### Technical References
- [DataMover Multi-Provider Integration](./docs/references/DataMover/DataMover-implementation.md) — multi-platform DataMover flow diagrams, platform-specific logic, and troubleshooting.
- [HCPEtcdBackup Implementation](./docs/references/HCPEtcdBackup/HCPEtcdBackup-implementation.md) — etcd backup CR orchestration details.

### Platform Examples
- [AWS](./examples/AWS/) — backup, restore, and configuration for AWS.
- [BareMetal](./examples/BareMetal/) — bare metal / agent deployments.
- [KubeVirt](./examples/kubevirt/) — KubeVirt platform.
- [OpenStack](./examples/Openstack/) — OpenStack platform.

## Dependency Management

The project validates compatibility with upstream dependencies via integration tests in `tests/integration/dependencies/`. The watched dependency `github.com/openshift/hypershift/api` is checked against the upstream main branch.

If the dependency validation test fails:

```bash
make update-deps
```

This parses the `watchedDependencies` map in `tests/integration/dependencies/dependencies_test.go`, updates each to the latest upstream commit, and runs `go mod tidy && go mod vendor`.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for how to submit changes, PR conventions, and testing requirements.

## License

Apache License 2.0 — see [LICENSE](./LICENSE).
