# Hypershift OADP Plugin

This velero/OADP plugin is designed to perform backup and restore of HostedControlPlanes in Openshift

## Quickstart

1. Deploy an OCP Management cluster with Hypershift running
2. Deploy OADP using the sample subscription (sample in `examples` folder)
3. Create the Cloud credentials for you storage provider to store the backups (In this case AWS)
```
kubectl create secret generic cloud-credentials \
    --namespace openshift-oadp \
    --from-file cloud=<AWS_CREDS_FILE>
```
4. Create the DataProtectionApplication (sample in `examples` folder)
5. Fill and create the backup manifest (sample in `examples` folder)
6. Check the Backup status.