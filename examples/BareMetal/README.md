# BareMetal Provider

This directory contains examples and configurations for the BareMetal provider, specifically focusing on the Agent provider implementation.

## Requirements

### Worker Node Readoption

When restoring a HostedCluster in the same Management Cluster, there is an important requirement regarding worker node readoption:

- The `InfraEnv` must be separated into a different namespace from the HostedCluster
- This separation is necessary to ensure the Assisted Installer DDBB is not removed which allows you a proper worker node readoption during the restore process
- If the InfraEnv is not separated into another namespace, the BM nodes should be reignited with the discovery ISO.

## Configuration Files

This directory contains the following configuration files:

- `02-dpa-bm.yaml`: Data Protection Application configuration for BareMetal
- `03-backup-bm-csi-compatible.yaml`: Backup configuration for CSI-compatible storage
- `03-backup-bm-csi-no-compatible.yaml`: Backup configuration for non-CSI-compatible storage
- `04-restore-bm-csi-compatible.yaml`: Restore configuration for CSI-compatible storage
- `04-restore-bm-csi-no-compatible.yaml`: Restore configuration for non-CSI-compatible storage

## Usage

1. Ensure the `InfraEnv` is in a separate namespace from the HostedCluster if you want to keep the nodes and avoid reprovision.
2. Apply the appropriate configuration files based on your storage setup
3. Follow the backup and restore procedures as documented
