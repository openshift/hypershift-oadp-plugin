# Hypershift OADP Plugin

This velero/OADP plugin is designed to perform backup and restore of HostedControlPlanes in Openshift

## Quickstart

1. Deploy an OCP Management cluster with Hypershift running
2. Deploy OADP using the sample subscription (sample in `examples` folder)
3. Create the Cloud credentials for you storage provider to store the backups (In this case AWS)
```
kubectl create secret generic cloud-credentials \
    --namespace openshift-adp \
    --from-file cloud=<AWS_CREDS_FILE>
```
4. Create the DataProtectionApplication (sample in `examples` folder)
5. Fill and create the backup manifest (sample in `examples` folder)
6. Check the Backup status.

## Documentation

For detailed technical documentation and implementation guides, please refer to the following resources:

### Core Features
- **[DataMover Multi-Provider Integration](./docs/references/DataMover/DataMover-implementation.md)** - Comprehensive guide covering the multi-platform DataMover implementation, including flow diagrams, platform-specific logic, and troubleshooting information.

### Examples
- **[AWS Examples](./examples/AWS/)** - Complete examples for AWS platform including backup, restore, and configuration files
- **[BareMetal Examples](./examples/BareMetal/)** - Examples for bare metal deployments
- **[KubeVirt Examples](./examples/kubevirt/)** - Examples for KubeVirt platform
- **[OpenStack Examples](./examples/Openstack/)** - Examples for OpenStack platform