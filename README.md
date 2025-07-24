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


## Testing

```sh
OADP_CRED_FILE=<path_to_backupLocations_credentials_file>
OADP_BUCKET=<bucket_name>
CI_CRED_FILE=<path_to_snapshotLocations_credentials_file>
VSL_REGION=<snapshotLocations_region>
# non required
BSL_REGION=<backupLocations_region>
OADP_TEST_NAMESPACE=<test_namespace>
OPENSHIFT_CI=false
SKIP_MUST_GATHER=true

TEST_HCP=true make test-e2e GINKGO_ARGS="--ginkgo.focus='HCP Backup and Restore tests'"
```
