package common

import (
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
)

// ResolveCredentialRef returns the BSL's explicit credential reference, or
// falls back to the well-known cloud-credentials secret used by standalone
// Velero installations (ARO with Workload Identity, future ROSA without DPA).
func ResolveCredentialRef(bsl *velerov1api.BackupStorageLocation) *corev1.SecretKeySelector {
	if bsl.Spec.Credential != nil {
		return bsl.Spec.Credential
	}
	return &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: DefaultCredentialSecretName},
		Key:                  DefaultCredentialSecretKey,
	}
}
