// Copyright 2019 Cloudbase Solutions Srl
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

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
