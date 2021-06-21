package db

import "path/filepath"

type TrackedDisk struct {
	TrackingID string
	Path       string
	Major      uint32
	Minor      uint32
}

// SnapStore is the database representation of a veeamsnap snap store.
// Veeamsnap supports creating one snap store for multiple devices, but
// we will be creating a separate snap store per device. It's easier to
// model. If it turns out to be a bad idea, we can switch later, as state
// does not really persist across reboots.
type SnapStore struct {
	SnapStoreID        string
	TrackedDisk        TrackedDisk
	StorageLocation    SnapStoreFilesLocation
	TotalAllocatedSize uint64
}

func (s SnapStore) Path() string {
	if s.SnapStoreID == "" {
		return ""
	}

	return filepath.Join(s.StorageLocation.Path, s.SnapStoreID)
}

type SnapStoreMapping struct {
	TrackingID             string
	TrackedDisk            TrackedDisk
	SnapStoreFilesLocation SnapStoreFilesLocation
}

// SnapStoreFilesLocation holds
type SnapStoreFilesLocation struct {
	TrackingID string
	// Path is the filesystem path where the snap store files will be
	// created. This must coincide with items in the CoWDestination
	// config value. This folder must be cleared of all pre-allocated
	// files, and re-initialized after a reboot. We (currently) cannot
	// persist CBT/tracking data across reboots, and any files left over
	// from previous sessions, will simply just take up space.
	// One more important aspect to understand, is that once allocated,
	// a range of extents cannot be removed. So any allocation of disk
	// space for CoW purposes, must be preserved until either the kernel
	// module is reloaded or the system is rebooted.
	Path string

	// Total capacity is the total amount of disk exposed by the
	// filesystem where we create files with which we allocate
	// extents.
	TotalCapacity uint64

	// DevicePath is the path in /dev/ of the device which holds the
	// location specified in Path. This can be a device mapper.
	DevicePath string

	// Major is the major number of the device which is mounted
	// in Path.
	Major uint32
	// Minor is the minor number of the device which is mounted
	// in Path.
	Minor uint32

	// Enabled indicates whether or not this location can be used to
	// allocate new extents. This is an administrative flag that allows
	// operators control over where allocations can and cannot be done.
	Enabled bool
}

// SnapStoreFile is a file that was pre-allocated on disk, the extents of
// which are passed to the veeamsnap kernel module to use as a CoW destination
// for blocks that change on tracked disks. The file itself serves as a
// mechanism to tell the filesystem not to write anything to those extents.
// The kernel module itself has no concept of files. It only cares about
// the device, and sectors it should have exclusivity to, in order to perform
// CoW operations when needed.
// WARNING: Removing the file, does not remove the extents already passed
// to the kernel module. Removing it, may cause corruption, as the filesystem
// now thinks those extents are free and can write files in those sectors.
// This can lead both to file corruption and snapshot corruption.
type SnapStoreFile struct {
	TrackingID             string
	SnapStore              SnapStore
	SnapStoreFilesLocation SnapStoreFilesLocation
	Path                   string
	Size                   uint64
}

type Image struct {
	TrackingID string
	Path       string
	Major      uint32
	Minor      uint32
}

type SnapshotImage struct {
	TrackingID string
	// DevicePath is the snapshot device path in /dev.
	DevicePath string
	Major      uint32
	Minor      uint32
	SnapshotID uint64
}

type VolumeSnapshot struct {
	TrackingID string
	// SnapshotNumber is the ID of the snapshot, as saved
	// in the CBT bitmap.
	SnapshotNumber uint32
	// GenerationID is the generation ID of this snapshot.
	GenerationID string

	// OriginalDevice is the device that was snapshot.
	OriginalDevice TrackedDisk
	// SnapshotImage is the resulting image that was created by the snapshot.
	SnapshotImage SnapshotImage

	Bitmap     []byte
	SnapshotID uint64
}

type Snapshot struct {
	// SnapshotID is the internal ID used to delete the snapshot
	// once we are done with it.
	SnapshotID uint64

	// VolumeSnapshots is an array of all the disk snapshots that
	// are included in this snapshot.
	VolumeSnapshots []VolumeSnapshot
}
