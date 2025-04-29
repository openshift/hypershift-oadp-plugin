# KubeVirt Backup/Restore sample objects

⚠️ **IMPORTANT**: These are just sample configurations associated with the provider marked in each folder. Before applying any configuration to your OpenShift cluster, please carefully review and adjust all parameters according to your specific environment and requirements.

This directory contains sample objects specifically for the KubeVirt provider. These examples demonstrate how to configure backup and restore operations in an KubeVirt environment.

## Important Notes

- This configuration is specifically designed for KubeVirt environments
- Node readoption is not currently supported. In case of node failures, reprovisioning of nodes will be required
- Please review the [RedHat official documentation](https://docs.okd.io/latest/backup_and_restore/application_backup_and_restore/installing/installing-oadp-mcg.html#installing-oadp-mcg) for detailed information on how OADP works
- Always validate the configuration before applying it to your environment
