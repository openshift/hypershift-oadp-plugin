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
	return core.NewBackupPlugin(logger.WithFields(
		logrus.Fields{
			"type": "core-backup",
		},
	))
}

func newHCPRestorePlugin(logger logrus.FieldLogger) (interface{}, error) {
	return core.NewRestorePlugin(logger.WithFields(
		logrus.Fields{
			"type": "core-restore",
		},
	))
}
