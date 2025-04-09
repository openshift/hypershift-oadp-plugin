# AWS Backup/Restore sample objects

⚠️ **IMPORTANT**: These are just sample configurations associated with the provider marked in each folder. Before applying any configuration to your OpenShift cluster, please carefully review and adjust all parameters according to your specific environment and requirements.

This directory contains sample objects specifically for the AWS provider. These examples demonstrate how to configure backup and restore operations in an AWS environment.

## Important Notes

- This configuration is specifically designed for AWS environments
- Node readoption is not currently supported. In case of node failures, reprovisioning of nodes will be required
- Please review the [RedHat official documentation](https://docs.okd.io/latest/backup_and_restore/application_backup_and_restore/installing/installing-oadp-mcg.html#installing-oadp-mcg) for detailed information on how OADP works
- Always validate the configuration before applying it to your environment

## Setting up S3 Bucket

To create an S3 bucket and apply the required policy for OADP, follow these steps:

1. Create a new S3 bucket:
   ```bash
   aws s3api create-bucket --bucket <your-bucket-name> --region <your-region>
   ```

2. Apply the bucket policy:
   ```bash
   aws s3api put-bucket-policy --bucket <your-bucket-name> --policy file://s3-bucket-policy.json
   ```

3. Enable versioning on the bucket:
   ```bash
   aws s3api put-bucket-versioning --bucket <your-bucket-name> --versioning-configuration Status=Enabled
   ```

Make sure to:
- Replace `<your-bucket-name>` with your desired bucket name
- Replace `<your-region>` with your AWS region
- The bucket name must be globally unique across all AWS accounts
- The bucket policy file (`velero-s3-bucket-policy.json`) is included in this directory and contains the necessary permissions for OADP operations
