package e2e

import (
	"time"

	"github.com/openshift/hypershift-oadp-plugin/test/e2e/lib/helpers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	runTimeClientForSuiteRun    crclient.Client
	dpaCR                       *helpers.DpaCustomResource
	knownFlake                  bool
	accumulatedTestLogs         []string
	kubernetesClientForSuiteRun *kubernetes.Clientset
	flakeAttempts               = 3
	kubeConfig                  *rest.Config
	namespace                   string
	bslCredFile                 string
	instanceName                string
	provider                    string
	vslCredFile                 string
	settings                    string
	artifact_dir                string
	skipMustGather              bool
)

type VerificationFunction func(crclient.Client, string) error

type BackupRestoreCase struct {
	Namespace         string
	Name              string
	BackupRestoreType helpers.BackupRestoreType
	PreBackupVerify   VerificationFunction
	PostRestoreVerify VerificationFunction
	SkipVerifyLogs    bool
	BackupTimeout     time.Duration
}

type ApplicationBackupRestoreCase struct {
	BackupRestoreCase
	ApplicationTemplate string
	PvcSuffixName       string
}

type HCPBackupRestoreCase struct {
	BackupRestoreCase
	Template string
	Provider string
}
