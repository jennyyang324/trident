// Copyright 2016 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"k8s.io/client-go/pkg/api/resource"
	"k8s.io/client-go/pkg/api/v1"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
)

// canPVMatchWithPVC verifies that the volumeSize and volumeAccessModes
// are capable of fulfilling the corresponding claimSize and claimAccessModes.
// For this to be true, volumeSize >= claimSize and every access mode in
// claimAccessModes must be present in volumeAccessModes.
// Note that this allows volumes to exceed the attributes requested by the
// claim; this is acceptable, though undesirable, and helps us avoid racing
// with the binder.
func canPVMatchWithPVC(pv *v1.PersistentVolume,
	claim *v1.PersistentVolumeClaim,
) bool {
	claimSize, _ := claim.Spec.Resources.Requests[v1.ResourceStorage]
	claimAccessModes := claim.Spec.AccessModes
	volumeAccessModes := pv.Spec.AccessModes
	volumeSize, ok := pv.Spec.Capacity[v1.ResourceStorage]
	if !ok {
		log.WithFields(log.Fields{
			"PV":  pv.Name,
			"PVC": claim.Name,
		}).Error("Kubernetes frontend detected a corrupted PV with no size.")
		return false
	}
	// Do the easy check first.  These should be whole numbers, so value
	// *should* be safe.
	if volumeSize.Value() < claimSize.Value() {
		return false
	}
	for _, claimMode := range claimAccessModes {
		found := false
		for _, volumeMode := range volumeAccessModes {
			if claimMode == volumeMode {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func getAnnotation(annotations map[string]string, key string) string {
	if val, ok := annotations[key]; ok {
		return val
	}
	return ""
}

// getVolumeConfig generates a NetApp DVP volume config from the specs pulled
// from the PVC.
func getVolumeConfig(
	accessModes []v1.PersistentVolumeAccessMode,
	name string,
	size resource.Quantity,
	annotations map[string]string,
) *storage.VolumeConfig {
	var accessMode config.AccessMode
	if len(accessModes) > 1 {
		accessMode = config.ReadWriteMany
	} else if len(accessModes) == 0 {
		accessMode = config.ModeAny //or config.ReadWriteMany?
	} else {
		accessMode = config.AccessMode(accessModes[0])
	}
	return &storage.VolumeConfig{
		Name:            name,
		Size:            fmt.Sprintf("%d", size.Value()),
		Protocol:        config.Protocol(getAnnotation(annotations, AnnProtocol)),
		SnapshotPolicy:  getAnnotation(annotations, AnnSnapshotPolicy),
		ExportPolicy:    getAnnotation(annotations, AnnExportPolicy),
		SnapshotDir:     getAnnotation(annotations, AnnSnapshotDir),
		UnixPermissions: getAnnotation(annotations, AnnUnixPermissions),
		StorageClass:    getAnnotation(annotations, AnnClass),
		AccessMode:      accessMode,
	}
}

func CreateNFSVolumeSource(volConfig *storage.VolumeConfig) *v1.NFSVolumeSource {
	return &v1.NFSVolumeSource{
		Server: volConfig.AccessInfo.NfsServerIP,
		Path:   volConfig.AccessInfo.NfsPath,
	}
}

func CreateISCSIVolumeSource(volConfig *storage.VolumeConfig) *v1.ISCSIVolumeSource {
	return &v1.ISCSIVolumeSource{
		TargetPortal:   volConfig.AccessInfo.IscsiTargetPortal,
		IQN:            volConfig.AccessInfo.IscsiTargetIQN,
		Lun:            volConfig.AccessInfo.IscsiLunNumber,
		ISCSIInterface: volConfig.AccessInfo.IscsiInterface,
		FSType:         "ext4",
	}
}
