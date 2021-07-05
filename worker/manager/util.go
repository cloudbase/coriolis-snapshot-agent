package manager

import (
	"coriolis-snapshot-agent/apiserver/params"
	"coriolis-snapshot-agent/db"
)

func internalVolumeSnapToParamvolumeSnap(vol db.VolumeSnapshot) params.VolumeSnapshot {
	return params.VolumeSnapshot{
		SnapshotNumber: vol.SnapshotNumber,
		GenerationID:   vol.GenerationID,
		OriginalDevice: params.TrackedDevice{
			TrackingID: vol.OriginalDevice.TrackingID,
			DevicePath: vol.OriginalDevice.Path,
			Major:      vol.OriginalDevice.Major,
			Minor:      vol.OriginalDevice.Minor,
		},
		SnapshotImage: params.SnapshotImage{
			DevicePath: vol.SnapshotImage.DevicePath,
			Major:      vol.SnapshotImage.Major,
			Minor:      vol.SnapshotImage.Minor,
		},
	}
}

func internalSnapToSnapResponse(snap db.Snapshot) params.SnapshotResponse {
	ret := params.SnapshotResponse{
		SnapshotID: snap.SnapshotID,
	}
	volSnaps := make([]params.VolumeSnapshot, len(snap.VolumeSnapshots))
	for idx, val := range snap.VolumeSnapshots {
		volSnaps[idx] = internalVolumeSnapToParamvolumeSnap(val)
	}
	ret.VolumeSnapshots = volSnaps
	return ret
}

func internalSnapStoreToParamsSnapStore(store db.SnapStore) params.SnapStoreResponse {
	return params.SnapStoreResponse{
		ID:                 store.SnapStoreID,
		TrackedDiskID:      store.TrackedDisk.TrackingID,
		StorageLocationID:  store.StorageLocation.Path,
		AllocatedDiskSpace: store.TotalAllocatedSize,
	}
}
