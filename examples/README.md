# Samples

⚠️ **IMPORTANT**: These are just sample configurations associated with the provider marked in each folder. Before applying any configuration to your OpenShift cluster, please carefully review and adjust all parameters according to your specific environment and requirements.


## Credentials for DPA

The credentials used by Velero for DPA (Data Protection Application) follow a structure similar to AWS credentials. These credentials are used to authenticate and access the backup storage location. The format typically includes access keys, secret keys, and other provider-specific authentication details.

This is how to create the credentials secrets in the OCP Management cluster:

```bash
oc create secret generic cloud-credentials --namespace openshift-adp --from-file cloud=credentials-velero
```

The `credentials-velero` file should contain the credentials in the following format:

```ini
[default]
aws_access_key_id=<EXAMPLE_ID>
aws_secret_access_key=<EXAMPLE_ACCESS_KEY>
```

Replace the example values with your actual credentials. The file should be created before running the `oc create secret` command.
