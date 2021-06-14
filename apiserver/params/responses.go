package params

import (
	"coriolis-veeam-bridge/internal/ioctl"

	"github.com/google/uuid"
)

type Volume struct {
	Path               string
	DeviceID           ioctl.DevID
	Snapshots          int32
	LastSnapshotNumber int32
	ActiveSnapshots    []VolumeSnapshot
}

type VolumeSnapshot struct {
	// DevicePath is the snapshot device path in /dev
	DevicePath string
	// DeviceID is the major:minor number of the snapshot image
	// created in /dev.
	DeviceID ioctl.DevID
	// SnapshotID is the internal ID used to delete the snapshot
	// once we are done with it.
	SnapshotID uuid.UUID
	// SnapshotNumber is the ID of the snapshot, as saved
	// in the CBT bitmap.
	SnapshotNumber int32
}
