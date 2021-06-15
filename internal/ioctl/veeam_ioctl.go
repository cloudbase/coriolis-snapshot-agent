package ioctl

const (
	VEEAM_SNAP = 'V'
	VEEAM_DEV  = "/dev/veeamsnap"
)

var (
	// Tracking
	IOCTL_TRACKING_ADD               uintptr = 1074288130
	IOCTL_TRACKING_REMOVE            uintptr = 1074288131
	IOCTL_TRACKING_COLLECT           uintptr = 1074550276
	IOCTL_TRACKING_BLOCK_SIZE        uintptr = 1074025989
	IOCTL_TRACKING_READ_CBT_BITMAP   uintptr = 2149078534
	IOCTL_TRACKING_MARK_DIRTY_BLOCKS uintptr = 2149078535

	// Snapshots
	IOCTL_SNAPSHOT_CREATE  uintptr = 1075074576
	IOCTL_SNAPSHOT_DESTROY uintptr = 2148029969
	IOCTL_SNAPSHOT_ERRNO   uintptr = 1074550290

	// Snap store
	IOCTL_SNAPSTORE_CREATE        uintptr = 2149865000
	IOCTL_SNAPSTORE_MEMORY        uintptr = 2149078570
	IOCTL_SNAPSTORE_FILE          uintptr = 2149340713
	IOCTL_SNAPSTORE_FILE_MULTIDEV uintptr = 2149865004
	IOCTL_SNAPSTORE_CLEANUP       uintptr = 1075336747

	// collect snapshot images
	IOCTL_COLLECT_SNAPSHOT_IMAGES uintptr = 1074550320

	// collect snapshot data location
	IOCTL_COLLECT_SNAPSHOTDATA_LOCATION_START    uintptr = 1075074624
	IOCTL_COLLECT_SNAPSHOTDATA_LOCATION_GET      uintptr = 1075074625
	IOCTL_COLLECT_SNAPSHOTDATA_LOCATION_COMPLETE uintptr = 2148030018

	// persistent CBT data parameter
	IOCTL_PERSISTENTCBT_DATA uintptr = 2148292168
)