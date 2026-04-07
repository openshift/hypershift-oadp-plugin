package common

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	veleroapiv1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	cr "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	k8sSAFilePath = DefaultK8sSAFilePath
)

func getMetadataAndAnnotations(item runtime.Unstructured) (metav1.Object, map[string]string, error) {
	metadata, err := meta.Accessor(item)
	if err != nil {
		return nil, nil, err
	}

	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	return metadata, annotations, nil
}

// GetClient creates a controller-runtime client for Kubernetes
func GetClient() (crclient.Client, error) {
	config, err := GetConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get kubernetes config: %w", err)
	}
	client, err := crclient.New(config, crclient.Options{Scheme: CustomScheme})
	if err != nil {
		return nil, fmt.Errorf("unable to get kubernetes client: %w", err)
	}
	return client, nil
}

// GetConfig retrieves the Kubernetes REST configuration using the client-go library.
func GetConfig() (*rest.Config, error) {
	cfg, err := cr.GetConfig()
	if err != nil {
		return nil, err
	}
	cfg.QPS = 200
	cfg.Burst = 300
	return cfg, nil
}



// GetCurrentNamespace reads the namespace from the Kubernetes service account
// token file and returns it as a string. The file is expected to be located at
// "/var/run/secrets/kubernetes.io/serviceaccount/namespace". If there is an error
// reading the file, it returns an empty string and the error.
func GetCurrentNamespace() (string, error) {
	namespaceFilePath := filepath.Join(k8sSAFilePath, "namespace")
	namespace, err := os.ReadFile(namespaceFilePath)
	if err != nil {
		return "", err
	}
	return string(namespace), nil
}

// MatchSuffixKind checks if the given kind string ends with any of the provided suffixes.
// It returns true if a match is found, otherwise it returns false.
func MatchSuffixKind(kind string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(kind, suffix) {
			return true
		}
	}
	return false

}

// AddAnnotation adds an annotation to the given metadata object.
// If the annotations map is nil, it initializes it before adding the annotation.
func AddAnnotation(metadata metav1.Object, key, value string) {
	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	metadata.SetAnnotations(annotations)
}

// RemoveAnnotation removes the annotation with the specified key from the given metadata object.
// If the annotations map is nil, the function returns without making any changes.
func RemoveAnnotation(metadata metav1.Object, key string) {
	annotations := metadata.GetAnnotations()
	if annotations == nil {
		return
	}
	delete(annotations, key)
	metadata.SetAnnotations(annotations)
}


// AddLabel adds a label with the specified key and value to the given metadata object.
// If the metadata object does not have any labels, a new map is created to store the label.
func AddLabel(metadata metav1.Object, key, value string) {
	labels := metadata.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	metadata.SetLabels(labels)
}

// RemoveLabel removes a label from the metadata of a Kubernetes object.
// If the label does not exist, the function does nothing.
func RemoveLabel(metadata metav1.Object, key string) {
	labels := metadata.GetLabels()
	if labels == nil {
		return
	}
	delete(labels, key)
	metadata.SetLabels(labels)
}

// GetHCP retrieves the first HostedControlPlane object from the provided list of namespaces.
// It iterates through the namespaces and attempts to list HostedControlPlane objects in each namespace.
// If a HostedControlPlane is found, it returns the first one encountered.
// If no HostedControlPlane is found in any of the namespaces, it returns an error.
func GetHCP(ctx context.Context, nsList []string, c crclient.Client, log logrus.FieldLogger) (*hyperv1.HostedControlPlane, error) {
	for _, ns := range nsList {
		hcpList := &hyperv1.HostedControlPlaneList{}
		if err := c.List(ctx, hcpList, crclient.InNamespace(ns)); err != nil {
			return nil, fmt.Errorf("error getting HostedControlPlane: %v", err)
		}

		if len(hcpList.Items) <= 0 {
			log.Infof("HostedControlPlane not found, retrying in the rest of the namespaces: %s", ns)
			continue
		}
		log.Infof("found hostedcontrolplane %s/%s", hcpList.Items[0].Namespace, hcpList.Items[0].Name)

		return &hcpList.Items[0], nil
	}
	return nil, fmt.Errorf("no HostedControlPlane found")
}

func GetHCPNamespace(name, namespace string) string {
	return fmt.Sprintf("%s-%s", namespace, name)
}

// GetHostedCluster finds the HostedCluster that owns the HCP by deriving
// its namespace and name from the HCP namespace convention: {hc-namespace}-{hc-name}.
func GetHostedCluster(ctx context.Context, c crclient.Client, includedNamespaces []string, hcpNamespace string) (*hyperv1.HostedCluster, error) {
	var errs []error
	for _, ns := range includedNamespaces {
		if ns == hcpNamespace {
			continue
		}
		hcList := &hyperv1.HostedClusterList{}
		if err := c.List(ctx, hcList, crclient.InNamespace(ns)); err != nil {
			errs = append(errs, fmt.Errorf("list HostedClusters in namespace %s: %w", ns, err))
			continue
		}
		for i := range hcList.Items {
			hc := &hcList.Items[i]
			if GetHCPNamespace(hc.Name, hc.Namespace) == hcpNamespace {
				return hc, nil
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return nil, nil
}

// ShouldEndPluginExecution checks if the plugin should end execution by verifying if the required
// Hypershift resources (HostedControlPlane and HostedCluster) exist in the cluster.
// Returns true if the plugin should end execution (i.e., if this is not a Hypershift cluster).
func ShouldEndPluginExecution(ctx context.Context, backup *veleroapiv1.Backup, c crclient.Client, log logrus.FieldLogger) (bool, error) {
	if len(backup.Spec.IncludedNamespaces) == 0 {
		return true, fmt.Errorf("no namespaces provided")
	}

	// Check for both short and full resource names
	for _, resource := range backup.Spec.IncludedResources {
		if strings.Contains(resource, "hostedcluster") ||
			strings.Contains(resource, "hostedcontrolplane") ||
			strings.Contains(resource, "nodepool") {
			return false, nil
		}
	}

	exists, err := CRDExists(ctx, "hostedcontrolplanes.hypershift.openshift.io", c)
	if err != nil {
		return true, fmt.Errorf("error checking for HostedControlPlane CRD: %v", err)
	}

	if exists {
		return false, nil
	}

	return true, fmt.Errorf("no HostedControlPlane CRD found")
}

func CRDExists(ctx context.Context, crdName string, c crclient.Client) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := c.Get(ctx, client.ObjectKey{Name: crdName}, crd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

