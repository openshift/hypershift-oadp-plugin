package main

import (
	"github.com/openshift/hypershift-oadp-plugin/pkg/core"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

func main() {
	framework.NewServer().
		RegisterBackupItemAction("hypershift-oadp-plugin/backup-item-action", newHCPBackupPlugin).
		RegisterRestoreItemAction("hypershift-oadp-plugin/restore-item-action", newHCPRestorePlugin).
		Serve()
}

func newHCPBackupPlugin(logger logrus.FieldLogger) (interface{}, error) {
	logger.Info("Initializing HCP Backup Plugin")
	return core.NewBackupPlugin(logger)
}

func newHCPRestorePlugin(logger logrus.FieldLogger) (interface{}, error) {
	logger.Info("Initializing HCP Restore Plugin")
	return core.NewRestorePlugin(logger)
}
