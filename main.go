package main

import (
	"github.com/openshift/hypershift-oadp-plugin/pkg/core"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

func configureLogger(logger logrus.FieldLogger) logrus.FieldLogger {
	return logger.WithFields(
		logrus.Fields{
			"type": "hcp-plugin",
		},
	)
}

func main() {
	framework.NewServer().
		RegisterBackupItemAction("hypershift-oadp-plugin/backup-item-action", newHCPBackupPlugin).
		RegisterRestoreItemAction("hypershift-oadp-plugin/restore-item-action", newHCPRestorePlugin).
		Serve()
}

func newHCPBackupPlugin(logger logrus.FieldLogger) (interface{}, error) {
	return core.NewBackupPlugin(configureLogger(logger))
}

func newHCPRestorePlugin(logger logrus.FieldLogger) (interface{}, error) {
	return core.NewRestorePlugin(configureLogger(logger))
}
