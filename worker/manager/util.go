package manager

import (
	"coriolis-veeam-bridge/apiserver/params"
	"coriolis-veeam-bridge/db"
	"coriolis-veeam-bridge/internal/storage"
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

// internalBlockVolumeToParamsBlockVolume converts an internal block volume
// to a params block volume. We copy the values because we want to be free to
// change de underlying storage implementation in the future, without changing
// the response the consumers get. I realize this is a premature abstraction,
// but I could not bring myself to add json tags to a structure inside the
// internal package, let alone return it as is to the API consumer.
func internalBlockVolumeToParamsBlockVolume(volume storage.BlockVolume) params.BlockVolume {
	ret := params.BlockVolume{
		Name:               volume.Name,
		Path:               volume.Path,
		PartitionTableType: volume.PartitionTableType,
		PartitionTableUUID: volume.PartitionTableUUID,
		Size:               volume.Size,
		LogicalSectorSize:  volume.LogicalSectorSize,
		PhysicalSectorSize: volume.PhysicalSectorSize,
		FilesystemType:     volume.FilesystemType,
		AlignmentOffset:    volume.AlignmentOffset,
		Major:              volume.Major,
		Minor:              volume.Minor,
		DeviceMapperSlaves: volume.DeviceMapperSlaves,
		IsVirtual:          volume.IsVirtual,
	}
	for _, val := range volume.Partitions {
		ret.Partitions = append(ret.Partitions, internalPartitionToParamsPartition(val))
	}
	return ret
}

func internalPartitionToParamsPartition(partition storage.Partition) params.Partition {
	return params.Partition{
		Name:            partition.Name,
		Path:            partition.Path,
		Sectors:         partition.Sectors,
		FilesystemUUID:  partition.FilesystemUUID,
		PartitionUUID:   partition.PartitionUUID,
		PartitionType:   partition.PartitionType,
		Label:           partition.Label,
		FilesystemType:  partition.FilesystemType,
		StartSector:     partition.StartSector,
		EndSector:       partition.EndSector,
		AlignmentOffset: partition.AlignmentOffset,
		Major:           partition.Major,
		Minor:           partition.Minor,
	}
}

func internalSnapStoreToParamsSnapStore(store db.SnapStore) params.SnapStoreResponse {
	return params.SnapStoreResponse{
		ID:                 store.SnapStoreID,
		TrackedDiskID:      store.TrackedDisk.TrackingID,
		StorageLocationID:  store.StorageLocation.Path,
		AllocatedDiskSpace: store.TotalAllocatedSize,
	}
}
