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

## Development

### Dependency Management

This project includes automated dependency validation to ensure compatibility with upstream dependencies. The validation is performed through integration tests located in `tests/integration/dependencies/`.

#### Dependency Validation Test

The dependency validation test (`dependencies_test.go`) automatically checks that watched dependencies are up-to-date with their respective upstream main branches. This helps prevent:
- Schema-related compatibility issues
- API version mismatches
- Runtime errors due to outdated dependencies

Currently watched dependencies:
- `github.com/openshift/hypershift/api` - Core HyperShift API definitions

#### Updating Dependencies

If the dependency validation test fails, you can update all watched dependencies automatically using:

```bash
make update-deps
```

This command will:
1. Parse the watched dependencies from the test file
2. Update each dependency to the latest commit from the main branch
3. Run `go mod tidy` and `go mod vendor` to clean up the dependency tree
4. Provide detailed output about which dependencies were updated

#### Manual Dependency Updates

For manual dependency updates, you can also run:

```bash
# Update a specific dependency
go get github.com/openshift/hypershift/api@main

# Clean up and vendor dependencies
go mod tidy && go mod vendor
```

#### Adding New Watched Dependencies

To add a new dependency to the validation process:

1. Add the dependency to the `watchedDependencies` map in `tests/integration/dependencies/dependencies_test.go`
2. The format is: `"module-path": "upstream-repo-url"`
3. The update script will automatically discover and update the new dependency

Example:
```go
var watchedDependencies = map[string]string{
    "github.com/openshift/hypershift/api": "https://github.com/openshift/hypershift",
    "github.com/example/new-module": "https://github.com/example/repo",
}
```