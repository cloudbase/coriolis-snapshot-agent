package internal

const (
	VEEAM_SNAP = 'V'
	VEEAM_DEV  = "/dev/veeamsnap"
)

type DevID struct {
	Major uint32
	Minor uint32
}

type CBTInfo struct {
	DevID        DevID
	DevCapacity  uint64
	CBTMapSize   uint32
	SnapNumber   byte
	GenerationID [16]byte
}

type TrackingCollect struct {
	Count   uint32
	CBTInfo [8]byte
}

type TrackingReadCBTBitmap struct {
	DevID  DevID
	Offset uint32
	Length uint32
	// Buff is a pointer to a *[]byte (char*)
	Buff *byte
}

type BlockRange struct {
	// Offset in sectors
	Offset uint64
	// Count in sectors
	Count uint64
}

type TrackingMarkDirtyBlocks struct {
	DevID      DevID
	Count      uint32
	BlockRange *BlockRange
}

// Snapshots

type SnapshotCreate struct {
	SnapshotID uint64
	Count      uint32
	DevID      [8]byte
}

type SnapshotErrno struct {
	DevID     DevID
	ErrorCode int32
}

type Range struct {
	Left  uint64
	Right uint64
}

// Snap store

type SnapStoreCreate struct {
	ID       [16]byte
	DevID    DevID
	Count    uint32
	DevIDPtr [8]byte
}

type SnapStoreMemoryLimit struct {
	ID   [16]byte
	Size uint64
}

type SnapStoreFileAdd struct {
	ID         [16]byte
	RangeCount uint32
	// Ranges is a pointer to a list(?) of []Range{}
	Ranges [8]byte
}

type SnapStoreFileAddMultidev struct {
	ID         [16]byte
	DevID      DevID
	RangeCount uint32
	// Ranges is a pointer to a list(?) of []Range{}
	Ranges [8]byte
}

type SnapStoreCleanup struct {
	ID          [16]byte
	FilledBytes uint64
}

// collect snapshot images
type ImageInfo struct {
	OriginalDevID DevID
	SnapshotDevID DevID
}

type CollectSnapshotImages struct {
	Count int32
	// ImageInfo is a pointer to an ImageInfo{} instance
	ImageInfo [8]byte
}

// collect snapshot data location
type CollectSnapshotdataLocationStart struct {
	DevID       DevID
	MagicLength int32
	// MagicBuff is a pointer to an interface{}
	MagicBuff [8]byte
}

type CollectSnapshotdataLocationGet struct {
	DevID      DevID
	RangeCount uint32
	// Ranges is a pointer to a list(?) of []Range{}
	Ranges [8]byte
}

type CollectSnapshotdataLocationComplete struct {
	DevID DevID
}

// persistent CBT data parameter
type PersistentCBTData struct {
	Size uint32
	// Parameter is a pointer to a char*
	Parameter [8]byte
}

var (
	// Tracking
	// IOCTL_TRACKING_ADD               = ioctl.IOW(uintptr(VEEAM_SNAP), 2, unsafe.Sizeof(DevID{}))
	IOCTL_TRACKING_ADD uintptr = 1074288130

	// IOCTL_TRACKING_REMOVE            = ioctl.IOW(uintptr(VEEAM_SNAP), 3, unsafe.Sizeof(DevID{}))
	IOCTL_TRACKING_REMOVE uintptr = 1074288131

	// IOCTL_TRACKING_COLLECT           = ioctl.IOW(uintptr(VEEAM_SNAP), 4, unsafe.Sizeof(TrackingCollect{}))
	IOCTL_TRACKING_COLLECT uintptr = 1074550276

	// IOCTL_TRACKING_BLOCK_SIZE        = ioctl.IOW(uintptr(VEEAM_SNAP), 5, 4)
	IOCTL_TRACKING_BLOCK_SIZE uintptr = 1074025989

	// IOCTL_TRACKING_READ_CBT_BITMAP   = ioctl.IOR(uintptr(VEEAM_SNAP), 6, unsafe.Sizeof(TrackingReadCBTBitmap{}))
	IOCTL_TRACKING_READ_CBT_BITMAP uintptr = 2149078534

	// IOCTL_TRACKING_MARK_DIRTY_BLOCKS = ioctl.IOR(uintptr(VEEAM_SNAP), 7, unsafe.Sizeof(TrackingMarkDirtyBlocks{}))
	IOCTL_TRACKING_MARK_DIRTY_BLOCKS uintptr = 2149078535

	// Snapshots
	IOCTL_SNAPSHOT_CREATE uintptr = 1075074576

	// IOCTL_SNAPSHOT_DESTROY         = ioctl.IOR(uintptr(VEEAM_SNAP), 0x11, 8)
	IOCTL_SNAPSHOT_DESTROY uintptr = 2148029969

	// IOCTL_SNAPSHOT_ERRNO = ioctl.IOW(uintptr(VEEAM_SNAP), 0x12, unsafe.Sizeof(SnapshotErrno{}))
	IOCTL_SNAPSHOT_ERRNO uintptr = 1074550290

	// Snap store
	// IOCTL_SNAPSTORE_CREATE         = ioctl.IOR(uintptr(VEEAM_SNAP), 0x28, unsafe.Sizeof(SnapStoreCreate{}))
	IOCTL_SNAPSTORE_CREATE uintptr = 2149865000

	// IOCTL_SNAPSTORE_MEMORY         = ioctl.IOR(uintptr(VEEAM_SNAP), 0x2A, unsafe.Sizeof(SnapStoreMemoryLimit{}))
	IOCTL_SNAPSTORE_MEMORY uintptr = 2149078570

	// IOCTL_SNAPSTORE_FILE = ioctl.IOR(uintptr(VEEAM_SNAP), 0x29, unsafe.Sizeof(SnapStoreFileAdd{}))
	IOCTL_SNAPSTORE_FILE uintptr = 2149340713

	// IOCTL_SNAPSTORE_FILE_MULTIDEV = ioctl.IOR(uintptr(VEEAM_SNAP), 0x2C, unsafe.Sizeof(SnapStoreFileAddMultidev{}))
	IOCTL_SNAPSTORE_FILE_MULTIDEV uintptr = 2149865004

	// IOCTL_SNAPSTORE_CLEANUP         = ioctl.IOW(uintptr(VEEAM_SNAP), 0x2B, unsafe.Sizeof(SnapStoreCleanup{}))
	IOCTL_SNAPSTORE_CLEANUP uintptr = 1075336747

	// collect snapshot images
	// IOCTL_COLLECT_SNAPSHOT_IMAGES = ioctl.IOW(uintptr(VEEAM_SNAP), 0x30, unsafe.Sizeof(CollectSnapshotImages{}))
	IOCTL_COLLECT_SNAPSHOT_IMAGES uintptr = 1074550320

	// collect snapshot data location
	// IOCTL_COLLECT_SNAPSHOTDATA_LOCATION_START            = ioctl.IOW(uintptr(VEEAM_SNAP), 0x40, unsafe.Sizeof(CollectSnapshotdataLocationStart{}))
	IOCTL_COLLECT_SNAPSHOTDATA_LOCATION_START uintptr = 1075074624

	// IOCTL_COLLECT_SNAPSHOTDATA_LOCATION_GET           = ioctl.IOW(uintptr(VEEAM_SNAP), 0x41, unsafe.Sizeof(CollectSnapshotdataLocationGet{}))
	IOCTL_COLLECT_SNAPSHOTDATA_LOCATION_GET uintptr = 1075074625

	// IOCTL_COLLECT_SNAPSHOTDATA_LOCATION_COMPLETE         = ioctl.IOR(uintptr(VEEAM_SNAP), 0x42, unsafe.Sizeof(CollectSnapshotdataLocationComplete{}))
	IOCTL_COLLECT_SNAPSHOTDATA_LOCATION_COMPLETE uintptr = 2148030018

	// persistent CBT data parameter
	// IOCTL_PERSISTENTCBT_DATA         = ioctl.IOR(uintptr(VEEAM_SNAP), 0x48, unsafe.Sizeof(PersistentCBTData{}))
	IOCTL_PERSISTENTCBT_DATA uintptr = 2148292168
)
