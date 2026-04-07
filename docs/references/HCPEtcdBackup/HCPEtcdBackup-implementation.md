# HCPEtcdBackup Integration

## Table of Contents
- [Overview](#overview)
- [Architecture](#architecture)
- [Backup Flow](#backup-flow)
- [Restore Flow](#restore-flow)
- [Configuration](#configuration)
- [Storage Layout](#storage-layout)
- [Credential Handling](#credential-handling)
- [Implementation Details](#implementation-details)
- [Dependencies](#dependencies)
- [Manual Testing](#manual-testing)
- [Troubleshooting](#troubleshooting)

## Overview

The HCPEtcdBackup integration adds an alternative etcd backup method to the OADP plugin. Instead of relying on CSI VolumeSnapshots or filesystem-level backups of etcd data volumes, it leverages the HyperShift Operator's `HCPEtcdBackup` controller to perform native etcd snapshots and upload them to object storage.

### Backup Methods

The plugin supports two mutually exclusive etcd backup methods, controlled by the `etcdBackupMethod` configuration key:

| Method | Value | Description |
|---|---|---|
| **Volume Snapshot** | `volumeSnapshot` (default) | Uses CSI VolumeSnapshots or FSBackup to capture etcd PVCs. This is the legacy behavior. |
| **Etcd Snapshot** | `etcdSnapshot` | Creates an `HCPEtcdBackup` CR that triggers a native `etcdctl snapshot save`, then uploads the snapshot to the same object store used by Velero. |

### Key Benefits of etcdSnapshot

- Produces a portable, self-contained etcd snapshot (`.db` file)
- No dependency on CSI drivers or storage-class-specific snapshot mechanisms
- Snapshot is stored alongside the Velero backup data in the BSL
- The snapshot URL is persisted in the HostedCluster status, surviving CR retention policies

## Architecture

### Components

```text
                    OADP Plugin (BackupPlugin)
                           │
            ┌──────────────┼──────────────┐
            │              │              │
    createEtcdBackup   Execute()    waitForCompletion
            │                             │
            ▼                             ▼
   ┌─────────────────┐          ┌──────────────────┐
   │  Orchestrator    │          │  Poll Condition   │
   │  - fetchBSL      │          │  - VerifyInProgress│
   │  - mapBSLToStorage│         │  - WaitForCompletion│
   │  - copyCredSecret│          └──────────────────┘
   │  - Create CR      │
   └────────┬──────────┘
            │
            ▼
   ┌─────────────────────┐
   │  HCPEtcdBackup CR   │  (in HCP namespace)
   └────────┬─────────────┘
            │
            ▼
   ┌─────────────────────┐
   │  HyperShift Operator │  (HCPEtcdBackup controller)
   │  - etcdctl snapshot  │
   │  - Upload to S3/Azure│
   │  - Update HC status  │
   └──────────────────────┘
```

### File Layout

| File | Purpose |
|---|---|
| `pkg/etcdbackup/orchestrator.go` | Core orchestration: BSL mapping, CR creation, polling, credential copy |
| `pkg/core/backup.go` | Backup plugin: etcd backup method routing, pod/PVC exclusion |
| `pkg/core/restore.go` | Restore plugin: snapshotURL injection into HostedCluster spec |
| `pkg/common/types.go` | Shared constants for backup methods, annotations, volume names |
| `pkg/common/scheme.go` | Scheme registration including apiextensionsv1 for CRD checks |

## Backup Flow

### Sequence

1. **Plugin initialization** (`NewBackupPlugin`): Reads `etcdBackupMethod` from the ConfigMap. Validates the value. Defaults to `volumeSnapshot`.

2. **HCP resolution**: On the first `Execute()` call, the plugin resolves the `HostedControlPlane` from the backup's included namespaces.

3. **HCPEtcdBackup creation** (etcdSnapshot only): Runs once, idempotent across all `Execute()` calls:
   - Checks that the `HCPEtcdBackup` CRD exists in the cluster (safenet)
   - Fetches the Velero `BackupStorageLocation` (BSL)
   - Maps BSL config to `HCPEtcdBackupStorage` (S3 or Azure Blob)
   - Copies the BSL credential Secret to the HO namespace, remapping the data key from `cloud` to `credentials`
   - Optionally sets encryption fields (KMS key ARN / Azure encryption key URL) from the HostedCluster spec
   - Creates the `HCPEtcdBackup` CR in the HCP namespace with a unique name (`oadp-{backup-name}-{random-4-chars}`)
   - Polls until the controller acknowledges the backup (InProgress or Succeeded)

4. **Wait for completion**: When the `HostedControlPlane` or `HostedCluster` item is processed, the plugin waits for the `HCPEtcdBackup` to reach a terminal state (succeeded or failed). Timeout: 10 minutes.

5. **Cleanup**: After completion, the copied credential Secret is deleted from the HO namespace.

6. **Pod exclusion**: Etcd pods are excluded from the backup entirely (`return nil, nil, nil`) to prevent CSI VolumeSnapshots or FSBackup of their volumes.

7. **PVC exclusion**: Etcd PVCs (names matching `data-etcd-*`) are excluded from the backup to prevent CSI snapshots.

### Ordering Independence

The `Execute()` method is called once per backed-up item, with no guaranteed ordering. The plugin handles this by:

- Creating the `HCPEtcdBackup` CR before the switch statement (after HCP resolution), so it runs regardless of which item arrives first
- Making creation idempotent: if the orchestrator already created a CR, subsequent calls are no-ops
- Calling `waitForEtcdBackupCompletion()` in both the HCP and HC cases — the wait is also idempotent (returns immediately after the first successful wait)

## Restore Flow

### Sequence

1. When the `HostedCluster` item is processed during restore, the plugin reads the etcd snapshot URL from the annotation `hypershift.openshift.io/etcd-snapshot-url`. This annotation is set during backup because Velero strips status fields from items during restore.

2. If the URL is present and the HC has managed etcd (`spec.etcd.managed != nil`), the plugin injects the URL into `spec.etcd.managed.storage.restoreSnapshotURL`.

3. The modified HC is written back to Velero's output, so when the HC is created in the target cluster, the HyperShift Operator uses the snapshot URL to restore etcd from the snapshot.

### Why an Annotation Instead of Status

Velero strips the `status` subresource from items during restore. The backup plugin also injects `lastSuccessfulEtcdBackupURL` into the HC status for observability, but the restore plugin reads the URL from the annotation `hypershift.openshift.io/etcd-snapshot-url` since that survives the restore process.

> **Note**: The `lastSuccessfulEtcdBackupURL` status field is also set via unstructured map access during backup for informational purposes, but the restore flow relies exclusively on the annotation.

## Configuration

### Plugin ConfigMap

The plugin reads its configuration from a ConfigMap named `hypershift-oadp-plugin-config` in the OADP namespace (typically `openshift-adp`).

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: hypershift-oadp-plugin-config
  namespace: openshift-adp
data:
  etcdBackupMethod: "etcdSnapshot"    # or "volumeSnapshot" (default)
  hoNamespace: "hypershift"           # HyperShift Operator namespace (default)
  migration: "true"                   # Enable migration mode (optional)
```

### Configuration Keys

| Key | Values | Default | Description |
|---|---|---|---|
| `etcdBackupMethod` | `volumeSnapshot`, `etcdSnapshot` | `volumeSnapshot` | Controls which etcd backup strategy is used |
| `hoNamespace` | any namespace name | `hypershift` | Namespace where the HyperShift Operator runs |
| `migration` | `true`, `false` | `false` | Enables migration-specific behavior (e.g., Agent platform PreserveOnDelete) |

### Generating the ConfigMap

A helper script is available at the project's documentation directory:

```bash
# Set etcdSnapshot method
./generate-plugin-config.sh -e etcdSnapshot

# Dry-run to review
./generate-plugin-config.sh -e etcdSnapshot -d

# Override all defaults
./generate-plugin-config.sh -n my-adp-ns -e etcdSnapshot -o my-ho-ns -m true
```

### Backup Manifest

When using `etcdSnapshot`, the Velero Backup manifest should disable volume-level backups since etcd data is handled by the HCPEtcdBackup controller:

```yaml
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: hcp-aws-backup
  namespace: openshift-adp
spec:
  storageLocation: default
  includedNamespaces:
  - clusters
  - clusters-<hosted-cluster-name>
  includedResources:
  - sa
  - role
  - rolebinding
  - pod
  - pvc
  - pv
  - configmap
  - secrets
  - services
  - deployments
  - statefulsets
  - hostedcluster
  - nodepool
  - hostedcontrolplane
  - cluster
  - awscluster
  - awsmachinetemplate
  - awsmachine
  - machinedeployment
  - machineset
  - machine
  - route
  - clusterdeployment
  - namespace
  snapshotMoveData: false
  defaultVolumesToFsBackup: false
  snapshotVolumes: false
```

## Storage Layout

The etcd snapshot is stored alongside the Velero backup data in the BSL, following Velero's directory convention:

```text
s3://<bucket>/<bsl-prefix>/backups/<backup-name>/etcd-backup/<timestamp>.db
```

Example:
```text
s3://my-oadp-bucket/backup-objects/backups/hcp-aws-backup/etcd-backup/1775575637.db
```

This ensures:
- The snapshot is co-located with the rest of the backup
- Velero does not flag `etcd-backup` as an invalid top-level directory (which would make the BSL unavailable)
- Backup retention policies applied to the Velero backup directory also cover the etcd snapshot

## Credential Handling

### BSL to HCPEtcdBackup Credential Flow

The HCPEtcdBackup controller needs credentials to upload the snapshot to object storage. The OADP plugin bridges the gap between Velero's BSL credentials and the controller's expectations:

1. **Source**: The BSL references a Secret via `spec.credential` (a `SecretKeySelector` with `name` and `key`, typically key = `cloud`)

2. **Copy**: The plugin copies the credential data to a new Secret in the HO namespace with:
   - Name: `etcd-backup-creds-<backup-name>`
   - Label: `hypershift.openshift.io/etcd-backup: "true"`
   - Key remapping: BSL key (e.g., `cloud`) is remapped to `credentials` (expected by the controller)

3. **Reuse**: If the destination Secret already exists, it is reused (STS credentials contain an IAM Role ARN that does not rotate)

4. **Cleanup**: After the backup completes (or fails), the copied Secret is deleted

### Key Remapping

The controller mounts the credential Secret as a volume at `/etc/etcd-backup-creds/` and reads the file `credentials`. Velero BSL Secrets typically store credentials under the key `cloud`. The plugin extracts only the referenced key and writes it as `credentials` in the destination Secret.

## Implementation Details

### CRD Existence Check

Before creating an `HCPEtcdBackup` CR, the plugin verifies that the CRD exists in the cluster. This is a safenet — if `etcdBackupMethod` is `etcdSnapshot` but the CRD is missing, the backup fails with a clear error rather than silently falling back.

The check requires `apiextensionsv1` to be registered in the client scheme (`pkg/common/scheme.go`).

### Polling

The plugin uses `wait.PollUntilContextTimeout` from `k8s.io/apimachinery/pkg/util/wait` to poll the `HCPEtcdBackup` status:

- **VerifyInProgress**: 30-second timeout, 5-second interval. Checks that the controller acknowledged the backup.
- **WaitForCompletion**: 10-minute timeout, 5-second interval. Waits for terminal state (succeeded or failed).

Both check the `BackupCompleted` condition on the CR.

### Unique CR Naming

Each backup creates an `HCPEtcdBackup` CR with a unique name: `oadp-<backup-name>-<4-char-random-suffix>`. This uses `k8s.io/apimachinery/pkg/util/rand.String(4)` and prevents collisions with previous backup runs.

## Dependencies

### HyperShift PRs

This feature depends on changes in the openshift/hypershift repository:

| PR | Description | Jira | Status |
|---|---|---|---|
| [#8139](https://github.com/openshift/hypershift/pull/8139) | HCPEtcdBackup controller | [CNTRLPLANE-2678](https://issues.redhat.com/browse/CNTRLPLANE-2678) | Pending merge |
| CNTRLPLANE-3173 | `LastSuccessfulEtcdBackupURL` field in HostedClusterStatus | [CNTRLPLANE-3173](https://issues.redhat.com/browse/CNTRLPLANE-3173) | Pending merge |

#### PR #8139 Dependency Chain (all merged)

The HCPEtcdBackup controller (PR #8139) depends on the following merged PRs:

| PR | Description |
|---|---|
| [#8010](https://github.com/openshift/hypershift/pull/8010) | `fetch-etcd-certs` CPO subcommand |
| [#8017](https://github.com/openshift/hypershift/pull/8017) | `etcd-upload` CPO subcommand |
| [#8040](https://github.com/openshift/hypershift/pull/8040) | `etcd-backup` CPO subcommand |
| [#8114](https://github.com/openshift/hypershift/pull/8114) | Transfer Manager upgrade |

### OADP Plugin PR

| PR | Description | Jira |
|---|---|---|
| This PR | Integrate HCPEtcdBackup lifecycle into OADP backup/restore flow | [CNTRLPLANE-2685](https://issues.redhat.com/browse/CNTRLPLANE-2685) |

### Enhancement

The overall design is defined in [Enhancement PR #1945](https://github.com/openshift/enhancements/pull/1945).

### Post-Merge Vendor Update

Once both HyperShift PRs are merged, the vendor must be updated to:

1. Replace `getLastSuccessfulEtcdBackupURL()` unstructured helper in `pkg/core/restore.go` with direct field access: `hc.Status.LastSuccessfulEtcdBackupURL`
2. Remove local constants (`BackupInProgressReason`, `BackupRejectedReason`, `EtcdBackupSucceeded`) in `pkg/common/types.go` in favor of the API-defined constants

## Manual Testing

### Prerequisites

- A management cluster with the HyperShift Operator running with the `HCPEtcdBackup` feature gate enabled
- OADP installed with a valid `BackupStorageLocation` (BSL)
- The OADP plugin image built and deployed with `etcdSnapshot` support
- A running HostedCluster

### Testing the Backup Flow

1. Create the plugin ConfigMap with `etcdSnapshot` method:

```bash
cat <<EOF | oc apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: hypershift-oadp-plugin-config
  namespace: openshift-adp
data:
  etcdBackupMethod: etcdSnapshot
  hoNamespace: <hypershift-operator-namespace>  # typically "hypershift"
EOF
```

2. Create a Velero Backup targeting the HostedCluster:

```yaml
apiVersion: velero.io/v1
kind: Backup
metadata:
  name: hcp-aws-backup
  namespace: openshift-adp
  labels:
    velero.io/storage-location: default
spec:
  storageLocation: default
  csiSnapshotTimeout: 10m0s
  includedNamespaces:
  - <hosted-cluster-namespace>
  - <hosted-cluster-namespace>-<hosted-cluster-name>
  includedResources:
  - sa
  - role
  - rolebinding
  - pod
  - configmap
  - priorityclasses
  - pdb
  - hostedcluster
  - nodepool
  - secrets
  - services
  - deployments
  - statefulsets
  - hostedcontrolplane
  - cluster
  - awscluster
  - awsmachinetemplate
  - awsmachine
  - machinedeployment
  - machineset
  - machine
  - route
  - clusterdeployment
  - namespace
  excludedResources: []
  itemOperationTimeout: 4h0m0s
  ttl: 2h0m0s
  snapshotMoveData: false
  defaultVolumesToFsBackup: false
  snapshotVolumes: false
```

3. Verify the HCPEtcdBackup CR was created in the HCP namespace:

```bash
oc get hcpetcdbackups -n <hosted-cluster-namespace>-<hosted-cluster-name>
```

4. Wait for the backup to complete and check the snapshot URL:

```bash
oc get hcpetcdbackup -n <hosted-cluster-namespace>-<hosted-cluster-name> \
  -o jsonpath='{.items[0].status.snapshotURL}'
```

5. Verify the snapshot was uploaded to the BSL bucket:

```bash
aws s3 ls s3://<bucket>/<prefix>/backups/hcp-aws-backup/etcd-backup/
```

6. Confirm the `lastSuccessfulEtcdBackupURL` is set on the HostedCluster status:

```bash
oc get hostedcluster <name> -n <hosted-cluster-namespace> \
  -o jsonpath='{.status.lastSuccessfulEtcdBackupURL}'
```

### Testing the Restore Flow

1. Delete the HostedCluster (or use a different management cluster) to simulate a disaster recovery scenario.

2. Create a Velero Restore from the backup:

```yaml
apiVersion: velero.io/v1
kind: Restore
metadata:
  name: hcp-aws-restore
  namespace: openshift-adp
spec:
  includedNamespaces:
  - <hosted-cluster-namespace>
  - <hosted-cluster-namespace>-<hosted-cluster-name>
  backupName: hcp-aws-backup
  cleanupBeforeRestore: CleanupRestored
  veleroManagedClustersBackupName: hcp-aws-backup
  veleroCredentialsBackupName: hcp-aws-backup
  veleroResourcesBackupName: hcp-aws-backup
  restorePVs: false
  preserveNodePorts: true
  existingResourcePolicy: update
  excludedResources:
  - nodes
  - events
  - events.events.k8s.io
  - backups.velero.io
  - restores.velero.io
  - resticrepositories.velero.io
```

3. Verify the restored HostedCluster has `restoreSnapshotURL` injected:

```bash
oc get hostedcluster <name> -n <hosted-cluster-namespace> \
  -o jsonpath='{.spec.etcd.managed.storage.restoreSnapshotURL}'
```

The output should be an array containing the snapshot URL, e.g.:
```
["s3://<bucket>/<prefix>/backups/hcp-aws-backup/etcd-backup/1775589976.db"]
```

### Manual restoreSnapshotURL Injection

To manually test the restore without going through the full OADP restore flow, you can patch the HostedCluster directly. Note that `restoreSnapshotURL` is an **array**, not a string:

```bash
# Correct — array syntax
oc patch hostedcluster <name> -n <hosted-cluster-namespace> --type=merge \
  -p '{"spec":{"etcd":{"managed":{"storage":{"restoreSnapshotURL":["s3://<bucket>/<prefix>/etcd-backup/snapshot.db"]}}}}}'

# Wrong — will be rejected by the API
oc patch hostedcluster <name> -n <hosted-cluster-namespace> --type=merge \
  -p '{"spec":{"etcd":{"managed":{"storage":{"restoreSnapshotURL":"s3://..."}}}}}'
```

> **Important**: The `restoreSnapshotURL` field only takes effect during HostedCluster bootstrap (initial etcd creation). Patching it on an already running cluster will not trigger an etcd restore. To test a full restore, the HostedCluster must be deleted and recreated via the OADP restore flow.

### Verifying volumeSnapshot Method is Unchanged

To confirm the default `volumeSnapshot` method still works correctly:

1. Remove or set `etcdBackupMethod: volumeSnapshot` in the ConfigMap
2. Run a backup and verify CSI VolumeSnapshots are created for etcd PVCs
3. Verify no `HCPEtcdBackup` CR is created

## Troubleshooting

### BSL Unavailable After Backup

**Symptom**: `BackupStorageLocation "default" is unavailable: Backup store contains invalid top-level directories`

**Cause**: An older version of the plugin stored the etcd snapshot at `<prefix>/etcd-backup/` instead of inside the backup directory.

**Fix**: Delete the orphaned directory from the bucket:
```bash
aws s3 rm s3://<bucket>/<prefix>/etcd-backup/ --recursive
```

### Credential Errors (IMDS / No Credentials Found)

**Symptom**: The etcd backup Job fails with `no EC2 IMDS role found` or similar credential errors.

**Cause**: The credential Secret was not remapped correctly, or an old Secret (with key `cloud` instead of `credentials`) is being reused.

**Fix**: Delete the stale credential Secret and retry:
```bash
oc delete secret -n hypershift -l hypershift.openshift.io/etcd-backup=true
```

### HCPEtcdBackup CRD Not Found

**Symptom**: `etcdBackupMethod is "etcdSnapshot" but HCPEtcdBackup CRD not found in the cluster`

**Cause**: The HyperShift Operator does not have the HCPEtcdBackup controller enabled (requires feature gate `HCPEtcdBackup`).

**Fix**: Enable the feature gate on the HyperShift Operator or switch to `volumeSnapshot` method.

### Backup Reuses Old HCPEtcdBackup

**Symptom**: The backup completes instantly without creating a new etcd snapshot, reusing a previous `snapshotURL`.

**Cause**: An old `HCPEtcdBackup` CR with a completed status still exists in the HCP namespace. Since v8, CR names include a random suffix to prevent this.

**Fix**: Delete old CRs before running a new backup:
```bash
oc delete hcpetcdbackups --all -n <hcp-namespace>
```

### Unknown Configuration Key Warning

**Symptom**: Velero logs show `unknown configuration key: etcdBackupMethod with value etcdSnapshot`

**Cause**: The plugin validator does not recognize the key. This was fixed to treat `etcdBackupMethod` and `hoNamespace` as known keys handled during plugin initialization.

**Fix**: Ensure you are running an updated plugin image that includes this fix.
