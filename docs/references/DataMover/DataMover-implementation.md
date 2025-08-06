# DataMover Multi-Provider Integration

## Table of Contents
- [Overview](#overview)
- [Architecture](#architecture)
- [Platform Support](#platform-support)
- [Flow Diagrams](#flow-diagrams)
- [Implementation Details](#implementation-details)
- [Configuration](#configuration)
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)

## Overview

The DataMover Multi-Provider Integration (PR [#91](https://github.com/openshift/hypershift-oadp-plugin/pull/91)) implements a comprehensive solution for managing data movement operations across different cloud platforms in the hypershift-oadp-plugin. This feature enables platform-specific validation and reconciliation logic to ensure reliable backup and restore operations.

### Key Features

- **Multi-Platform Support**: Native support for AWS, Azure, IBM Cloud, KubeVirt, OpenStack, Agent, and None platforms
- **Platform-Specific Logic**: Different reconciliation strategies optimized for each platform's capabilities
- **State Management**: Robust tracking of backup progress with boolean flags
- **Timeout Handling**: Configurable timeouts for DataUpload operations
- **High Availability**: HA-aware reconciliation logic for production environments
- **Backward Compatibility**: Maintains existing PodVolumeBackup logic for fsBackup scenarios

## Architecture

The DataMover integration follows a modular architecture with clear separation of concerns:

### Core Components

1. **BackupPluginValidator**: Main validation interface
2. **Platform-Specific Handlers**: Custom logic for each supported platform
3. **Reconciliation Engine**: Manages resource state and progress tracking
4. **Resource Monitors**: Track VolumeSnapshotContent, VolumeSnapshot, and DataUpload resources

### State Management

The system uses boolean flags to track progress:
- `PVBackupStarted`: Indicates if PV backup has begun
- `PVBackupFinished`: Indicates if PV backup is complete
- `DUStarted`: Indicates if DataUpload has begun
- `DUFinished`: Indicates if DataUpload is complete

## Platform Support

### Supported Platforms

| Platform | PV Required | DU Required | Reconciliation Strategy |
|----------|-------------|-------------|------------------------|
| AWS | ✅ | ✅ | Standard |
| Azure | ✅ | ❌ | Azure-specific |
| IBM Cloud | ✅ | ✅ | Standard |
| KubeVirt | ✅ | ✅ | Standard |
| OpenStack | ✅ | ✅ | Standard |
| Agent | ✅ | ✅ | Standard |
| None | ✅ | ✅ | Standard |

### Platform-Specific Requirements

- **Azure**: Only requires VolumeSnapshot operations (no DataUpload)
- **Other Platforms**: Require both VolumeSnapshot and DataUpload operations
- **All Platforms**: Support high availability configurations

## Flow Diagrams

### Main Backup Flow

The primary backup flow determines whether to use DataMover or traditional PodVolumeBackup logic:

```mermaid
flowchart TD
    A[Backup Process Starts] --> B{DefaultVolumesToFsBackup?}

    B -->|false| C[ValidateDataMover]
    B -->|true| D[PodVolumeBackup Logic]

    C --> E{Platform Type}

    E -->|AWS| F[AWS DataMover Logic]
    E -->|Azure| G[Azure DataMover Logic]
    E -->|IBM Cloud| H[IBM Cloud DataMover Logic]
    E -->|KubeVirt| I[KubeVirt DataMover Logic]
    E -->|OpenStack| J[OpenStack DataMover Logic]
    E -->|Agent/None| K[Agent/None DataMover Logic]
    E -->|Unsupported| L[Return Error]

    F --> M{PV & DU Finished?}
    G --> N{PV Finished?}
    H --> O{PV & DU Finished?}
    I --> P{PV & DU Finished?}
    J --> Q{PV & DU Finished?}
    K --> R{PV & DU Finished?}

    M -->|Yes| S[Return Success]
    M -->|No| T[reconcileStandardDataMover]
    N -->|Yes| U[Return Success]
    N -->|No| V[reconcileAzureDataMover]
    O -->|Yes| S
    O -->|No| T
    P -->|Yes| S
    P -->|No| T
    Q -->|Yes| S
    Q -->|No| T
    R -->|Yes| S
    R -->|No| T

    T --> W[Reconcile VolumeSnapshotContent]
    V --> X[Reconcile VolumeSnapshotContent]

    W --> Y{VSC Finished?}
    X --> Z{VSC Finished?}

    Y -->|No| AA[Return - Wait]
    Y -->|Yes| BB[Reconcile VolumeSnapshots]
    Z -->|No| CC[Return - Wait]
    Z -->|Yes| DD[Reconcile VolumeSnapshots]

    BB --> EE{VS Finished?}
    DD --> FF{VS Finished?}

    EE -->|No| GG[Return - Wait]
    EE -->|Yes| HH[Reconcile DataUpload]
    FF -->|No| II[Return - Wait]
    FF -->|Yes| JJ[Return Success - Azure Only]

    HH --> KK{DU Finished?}

    KK -->|No| LL[Return - Wait]
    KK -->|Yes| MM[Return Success]

    D --> NN[Check PodVolumeBackup]
    NN --> OO{PVB Started?}
    OO -->|No| PP[Check if PVB exists]
    OO -->|Yes| QQ{PVB Finished?}
    QQ -->|No| RR[Wait for PVB completion]
    QQ -->|Yes| SS[Continue with backup]
    PP --> TT{PVB Found?}
    TT -->|Yes| UU[Set PVB Started = true]
    TT -->|No| VV[Continue with backup]
    UU --> WW[Set PVB Finished = true]
    RR --> XX[Set PVB Finished = true]
    WW --> SS
    XX --> SS
    VV --> SS
```

### Reconciliation Strategies

The system implements two main reconciliation strategies:

#### Standard DataMover Reconciliation

Used by most platforms (AWS, IBM Cloud, KubeVirt, OpenStack, Agent, None):

```mermaid
flowchart TD
    A[reconcileStandardDataMover] --> B[ReconcileVolumeSnapshotContent]
    B --> C{VSC Finished?}
    C -->|No| D[Return - Wait]
    C -->|Yes| E[ReconcileVolumeSnapshots]
    E --> F{VS Finished?}
    F -->|No| G[Return - Wait]
    F -->|Yes| H[ReconcileDataUpload]
    H --> I{DU Finished?}
    I -->|No| J[Return - Wait]
    I -->|Yes| K[Return Success]
```

#### Azure DataMover Reconciliation

Azure-specific reconciliation (no DataUpload required):

```mermaid
flowchart TD
    L[reconcileAzureDataMover] --> M[ReconcileVolumeSnapshotContent]
    M --> N{VSC Finished?}
    N -->|No| O[Return - Wait]
    N -->|Yes| P[ReconcileVolumeSnapshots]
    P --> Q{VS Finished?}
    Q -->|No| R[Return - Wait]
    Q -->|Yes| S[Return Success - No DU for Azure]
```

### Resource Reconciliation Details

#### VolumeSnapshotContent Reconciliation

Manages the lifecycle of VolumeSnapshotContent resources:

```mermaid
flowchart TD
    A[ReconcileVolumeSnapshotContent] --> B{PV Backup Finished?}
    B -->|Yes| C[Return True]
    B -->|No| D{PV Backup Started?}
    D -->|No| E[CheckVolumeSnapshotContent]
    D -->|Yes| F{PV Backup Finished?}
    F -->|No| G[WaitForVolumeSnapshotContent]
    F -->|Yes| H[Return True]

    E --> I{VSC Found & Ready?}
    I -->|Yes| J[Set PV Started = true]
    I -->|No| K[Return False]
    J --> L[Set VSC Finished = true]
    L --> M[Return True]

    G --> N{VSC Ready?}
    N -->|Yes| O[Set VSC Finished = true]
    N -->|No| P[Wait & Retry]
    O --> Q[Return True]
    P --> R[Timeout?]
    R -->|Yes| S[Return Error]
    R -->|No| N
```

#### VolumeSnapshot Reconciliation

Handles VolumeSnapshot resource monitoring:

```mermaid
flowchart TD
    A[ReconcileVolumeSnapshots] --> B{PV Backup Finished?}
    B -->|Yes| C[Return True]
    B -->|No| D{PV Backup Started?}
    D -->|No| E[CheckVolumeSnapshot]
    D -->|Yes| F{PV Backup Finished?}
    F -->|No| G[WaitForVolumeSnapshot]
    F -->|Yes| H[Return True]

    E --> I{VS Found & Ready?}
    I -->|Yes| J[Set PV Started = true]
    I -->|No| K[Return False]
    J --> L[Set VS Finished = true]
    L --> M[Return True]

    G --> N{VS Ready?}
    N -->|Yes| O[Set VS Finished = true]
    N -->|No| P[Wait & Retry]
    O --> Q[Return True]
    P --> R[Timeout?]
    R -->|Yes| S[Return Error]
    R -->|No| N
```

#### DataUpload Reconciliation

Manages DataUpload resource lifecycle (not used for Azure):

```mermaid
flowchart TD
    A[ReconcileDataUpload] --> B{DU Finished?}
    B -->|Yes| C[Return True]
    B -->|No| D{DU Started?}
    D -->|No| E[CheckDataUpload]
    D -->|Yes| F{DU Finished?}
    F -->|No| G[WaitForDataUpload]
    F -->|Yes| H[Return True]

    E --> I{DU Found & Completed?}
    I -->|Yes| J[Set DU Started = true]
    I -->|No| K[Return False]
    J --> L[Set DU Finished = true]
    L --> M[Return True]

    G --> N{DU Completed?}
    N -->|Yes| O[Set DU Finished = true]
    N -->|No| P[Wait & Retry]
    O --> Q[Return True]
    P --> R[Timeout?]
    R -->|Yes| S[Return Error]
    R -->|No| N
```

### Platform-Specific Logic Flow

Shows how different platforms are handled:

```mermaid
flowchart TD
    A[Platform Type] --> B{Platform}

    B -->|AWS| C[Both PV & DU Required]
    B -->|Azure| D[Only PV Required]
    B -->|IBM Cloud| E[Both PV & DU Required]
    B -->|KubeVirt| F[Both PV & DU Required]
    B -->|OpenStack| G[Both PV & DU Required]
    B -->|Agent/None| H[Both PV & DU Required]

    C --> I[Check PV & DU Finished]
    D --> J[Check PV Finished Only]
    E --> K[Check PV & DU Finished]
    F --> L[Check PV & DU Finished]
    G --> M[Check PV & DU Finished]
    H --> N[Check PV & DU Finished]

    I -->|Both True| O[Return Success]
    I -->|Either False| P[Reconcile Standard]
    J -->|True| Q[Return Success]
    J -->|False| R[Reconcile Azure]
    K -->|Both True| S[Return Success]
    K -->|Either False| T[Reconcile Standard]
    L -->|Both True| U[Return Success]
    L -->|Either False| V[Reconcile Standard]
    M -->|Both True| W[Return Success]
    M -->|Either False| X[Reconcile Standard]
    N -->|Both True| Y[Return Success]
    N -->|Either False| Z[Reconcile Standard]
```

## Implementation Details

### Core Functions

#### ValidateDataMover
```go
func (p *BackupPluginValidator) ValidateDataMover(ctx context.Context, hcp *hyperv1.HostedControlPlane, backup *velerov1.Backup) error
```

Main entry point that:
- Validates platform type
- Checks current state
- Routes to appropriate reconciliation strategy
- Manages timeout and error handling

#### Reconciliation Functions

**Standard DataMover:**
```go
func (p *BackupPluginValidator) reconcileStandardDataMover(ctx context.Context, hcp *hyperv1.HostedControlPlane) error
```

**Azure DataMover:**
```go
func (p *BackupPluginValidator) reconcileAzureDataMover(ctx context.Context, hcp *hyperv1.HostedControlPlane) error
```

### Resource Monitoring

The system monitors three key resource types:

1. **VolumeSnapshotContent**: Kubernetes snapshot content resources
2. **VolumeSnapshot**: Kubernetes snapshot resources
3. **DataUpload**: Velero v2alpha1 data upload resources

### State Management

State is tracked using boolean pointers:
- `PVBackupStarted`: Tracks if PV backup has initiated
- `PVBackupFinished`: Tracks if PV backup is complete
- `DUStarted`: Tracks if DataUpload has initiated
- `DUFinished`: Tracks if DataUpload is complete

## Configuration

### Plugin Configuration

The DataMover supports several configuration options:

```yaml
dataUploadTimeout: "15"      # Timeout in minutes for DataUpload operations
dataUploadCheckPace: "30"    # Check interval in seconds
migration: "false"           # Migration mode flag
readoptNodes: "false"        # Node readoption flag
managedServices: "false"     # Managed services flag
```

### High Availability Configuration

For HA environments, set the `HA` flag to `true` in the validator:

```go
validator := &BackupPluginValidator{
    HA: true,
    // ... other configuration
}
```

## Testing

### Test Coverage

The implementation includes comprehensive test coverage:

#### Unit Tests
- **Platform Validation**: Tests for all supported platforms
- **State Management**: Tests for finished vs in-progress states
- **Edge Cases**: Tests for nil values and unsupported platforms
- **High Availability**: Tests for HA scenarios
- **Timeout Configuration**: Tests for different timeout settings

#### Integration Tests
- **Realistic Reconciliation**: Tests with actual Kubernetes resources
- **Resource Monitoring**: Tests for VolumeSnapshotContent, VolumeSnapshot, and DataUpload
- **Error Handling**: Tests for timeout and error scenarios

### Test Structure

```go
func TestValidateDataMoverPlatformValidation(t *testing.T)
func TestValidateDataMoverWithDifferentPlatforms(t *testing.T)
func TestValidateDataMoverWithFinishedStates(t *testing.T)
func TestValidateDataMoverWithClient(t *testing.T)
func TestValidateDataMoverEdgeCases(t *testing.T)
func TestValidateDataMoverWithHighAvailability(t *testing.T)
func TestValidateDataMoverWithDifferentTimeouts(t *testing.T)
func TestValidateDataMover_AWS_Reconciliation(t *testing.T)
```

## Troubleshooting

### Common Issues

#### Timeout Errors
- **Symptom**: `context deadline exceeded` errors
- **Solution**: Increase `dataUploadTimeout` configuration
- **Check**: Verify network connectivity and resource availability

#### Platform Validation Errors
- **Symptom**: `unsupported platform type` errors
- **Solution**: Verify platform type in HostedControlPlane spec
- **Check**: Ensure platform is in supported list

#### Resource Not Found
- **Symptom**: VolumeSnapshotContent or VolumeSnapshot not found
- **Solution**: Verify CSI snapshot controller is installed
- **Check**: Ensure snapshot resources are being created

#### State Management Issues
- **Symptom**: Infinite loops or stuck reconciliation
- **Solution**: Check boolean flag initialization
- **Check**: Verify resource status and conditions

### Debugging

Enable debug logging to troubleshoot issues:

```go
log.SetLevel(logrus.DebugLevel)
```

### Monitoring

Monitor these key metrics:
- Reconciliation duration
- Resource completion rates
- Error rates by platform
- Timeout occurrences

### Log Analysis

Key log messages to monitor:
- `"Reconciling standard data mover for HCP"`
- `"VolumeSnapshotContent is done"`
- `"VolumeSnapshot is done"`
- `"DataUpload is done"`
- `"error reconciling"`

## Conclusion

The DataMover Multi-Provider Integration provides a robust, scalable solution for managing data movement operations across different cloud platforms. The implementation ensures reliable backup and restore operations while maintaining backward compatibility and supporting high availability configurations.

The comprehensive test coverage and detailed flow diagrams make this implementation suitable for production environments across all supported platforms.