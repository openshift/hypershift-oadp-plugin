package main

import (
	"github.com/openshift/hypershift-oadp-plugin/pkg/core"
	//"github.com/openshift/hypershift-oadp-plugin/pkg/volumes"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

func main() {
	framework.NewServer().
		RegisterBackupItemAction("hypershift.openshift.io/common-backup-plugin", newCommonBackupPlugin).
		//RegisterRestoreItemAction("hypershift.openshift.io/common-restore-plugin", newCommonRestorePlugin).
		//RegisterBackupItemAction("hypershift.openshift.io/volumes-backup-plugin", newVolumesBackupPlugin).
		//RegisterRestoreItemAction("hypershift.openshift.io/volumes-restore-plugin", newVolumesRestorePlugin).
		Serve()
}

func newCommonBackupPlugin(logger logrus.FieldLogger) (interface{}, error) {
	return core.NewBackupPlugin(logger), nil
}

//func newCommonRestorePlugin(logger logrus.FieldLogger) (interface{}, error) {
//	return plugcommon.NewRestorePlugin(logger), nil
//}
//
//func newVolumesBackupPlugin(logger logrus.FieldLogger) (interface{}, error) {
//	return plugvols.NewBackupPlugin(logger), nil
//}
//
//func newVolumesRestorePlugin(logger logrus.FieldLogger) (interface{}, error) {
//	return plugvols.NewRestorePlugin(logger), nil
//}
