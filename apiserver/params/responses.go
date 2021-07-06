package params

type BackupType string

const (
	BackupTypeFull        BackupType = "full"
	BackupTypeIncremental BackupType = "incremental"
)

var (
	// NotFoundResponse is returned when a resource is not found
	NotFoundResponse = APIErrorResponse{
		Error:   "Not Found",
		Details: "The resource you are looking for was not found",
	}
	// UnauthorizedResponse is a canned response for unauthorized access
	UnauthorizedResponse = APIErrorResponse{
		Error:   "Not Authorized",
		Details: "You do not have the required permissions to access this resource",
	}
)

// LoginResponse is the response clients get on successful login.
type LoginResponse struct {
	Token string `json:"token"`
}

// ErrorResponse holds any errors generated during
// a request
type ErrorResponse struct {
	Errors map[string]string
}

// APIErrorResponse holds information about an error, returned by the API
type APIErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details"`
}

// Partition holds the information about a particular partition
type Partition struct {
	// Name is the name of the partition (sda1, sdb2, etc)
	Name string `json:"name"`
	// Path is the full path for this disk.
	Path string `json:"path,omitempty"`
	// Sectors represents the size of this partitions in sectors
	// you can find the size of the partition by multiplying this
	// with the logical sector size of the disk
	Sectors int `json:"sectors"`
	// FilesystemUUID represents the filesystem UUID of this partition
	FilesystemUUID string `json:"filesystem_uuid,omitempty"`
	// PartitionUUID is the UUID of the partition. On disks with DOS partition
	// tables, the partition UUID is made up of the partition table UUID and
	// the index of the partition. This means that if the partition table has
	// am UUID of "1e21670f", then sda1 (for example) will have a partition UUID
	// of "1e21670f-01". On GPT partition tables the UUID of the partition table
	// and that of partitions are proper UUID4, and are unique.
	PartitionUUID string `json:"partition_uuid,omitempty"`
	// PartitionType represents the partition type. For information about GPT
	// partition types, consult:
	// https://en.wikipedia.org/wiki/GUID_Partition_Table#Partition_type_GUIDs
	//
	// For information about MBR partition types, consult:
	// https://www.win.tue.nl/~aeb/partitions/partition_types-1.html
	PartitionType string `json:"partition_type,omitempty"`
	// Label is the FileSystem label
	Label string `json:"label,omitempty"`
	// FilesystemType represents the name of the filesystem this
	// partition is formatted with (xfs, ext4, ntfs, etc).
	// NOTE: this may yield false positives. libblkid returns ext4
	// for the Windows Reserved partition. The FS prober returns a
	// false positive, so take this with a grain of salt.
	FilesystemType string `json:"filesystem_type,omitempty"`
	// StartSector represents the sector at which the partition starts
	StartSector int `json:"start_sector"`
	// EndSector represents the last sector of the disk for this partition
	EndSector int `json:"end_sector,omitempty"`
	// AlignmentOffset indicates how many bytes the beginning of the device is
	// offset from the disk's natural alignment. For details, see:
	// https://www.kernel.org/doc/Documentation/ABI/testing/sysfs-block
	AlignmentOffset int `json:"alignment_offset"`
	// Major is the device node major number
	Major uint32 `json:"device_major"`
	// Minor is the device minor number
	Minor uint32 `json:"device_minor"`
}

type BlockVolume struct {
	// TrackingID is the DB tracking Id added for this disk. A value of -1
	// means it's not tracked.
	TrackingID string `json:"id,omitempty"`
	// Path is the full path for this disk.
	Path string `json:"path"`
	// PartitionTableType is the partition table type
	PartitionTableType string `json:"partition_table_type,omitempty"`
	// PartitionTableUUID represents the UUID of the partition table
	PartitionTableUUID string `json:"partition_table_uuid,omitempty"`
	// Name is just the device name, without the leading /dev
	Name string `json:"name,omitempty"`
	// Size is the size in bytes of this disk
	Size int64 `json:"size,omitempty"`
	// LogicalSectorSize  is the size of the sector reported by the operating system
	// for this disk. Usually this is 512 bytes
	LogicalSectorSize int64 `json:"logical_sector_size,omitempty"`
	// PhysicalSectorSize is the sector size reported by the disk. Some disks may have a
	// 4k sector size.
	PhysicalSectorSize int64 `json:"physical_sector_size,omitempty"`
	// Partitions is a list of discovered partition on this disk. This is the primary
	// source of truth when identifying disks
	Partitions []Partition `json:"partitions,omitempty"`
	// FilesystemType represents the name of the filesystem this
	// disk is formatted with (xfs, ext4, ntfs, etc). There are situations
	// when a whole disk is formatted with a particular FS.
	// NOTE: this may yield false positives. libblkid returns ext4
	// for the Windows Reserved partition. The FS prober returns a
	// false positive, so take this with a grain of salt.
	FilesystemType string `json:"filesystem_type"`
	// AlignmentOffset indicates how many bytes the beginning of the device is
	// offset from the disk's natural alignment. For details, see:
	// https://www.kernel.org/doc/Documentation/ABI/testing/sysfs-block
	AlignmentOffset int `json:"alignment_offset"`
	// Major is the device node major number
	Major uint32 `json:"device_major,omitempty"`
	// Minor is the device minor number
	Minor uint32 `json:"device_minor,omitempty"`

	// DeviceMapperSlaves holds the block device(s) that back this device
	DeviceMapperSlaves []string `json:"device_slaves,omitempty"`

	// IsVirtual specifies if this device is a virtual device.
	IsVirtual bool `json:"is_virtual"`
}

type SnapStoreLocation struct {
	// ID string `json:"id"`
	// AvailableCapacity is the amount of free disk space
	// on a device. This value can be different from
	// TotalCapacity - AllocatedCapacity, as there is no guarantee
	// that we will have sole access to the device.
	AvailableCapacity uint64 `json:"available_capacity"`
	// AllocatedCapacity is the amount of disk space that has
	// been allocated to snap stores. This value is calculated by
	// summing up the amount of disk space allocated by each file
	// that this service keeps track of.
	AllocatedCapacity uint64 `json:"allocated_capacity"`
	// TotalCapacity is the total amount of disk space a mount
	// point has.
	TotalCapacity uint64 `json:"total_capacity"`
	// Path is the path on the filesystem to the folder where
	// snap store storage is allocated.
	Path string `json:"path"`
	// DevicePath is the device in /dev which the folder represented
	// by Path is stored in.
	DevicePath string `json:"device_path"`
	// Major is the major number of the device which is mounted
	// in Path.
	Major uint32 `json:"major"`
	// Minor is the minor number of the device which is mounted
	// in Path.
	Minor uint32 `json:"minor"`
}

type SnapStoreResponse struct {
	ID                 string `json:"id"`
	TrackedDiskID      string `json:"tracked_disk_id"`
	StorageLocationID  string `json:"storage_location"`
	AllocatedDiskSpace uint64 `json:"allocated_disk_space"`
	StorageUsage       uint64 `json:"used_disk_space"`
}

type SnapStoreMappingResponse struct {
	ID                string `json:"id"`
	TrackedDiskID     string `json:"tracked_disk_id"`
	StorageLocationID string `json:"storage_location"`
}

// type Volume struct {
// 	Path               string
// 	DeviceID           types.DevID
// 	Snapshots          int32
// 	LastSnapshotNumber int32
// 	ActiveSnapshots    []VolumeSnapshot
// }

type SnapshotImage struct {
	// DevicePath is the snapshot device path in /dev.
	DevicePath string
	Major      uint32
	Minor      uint32
}

type TrackedDevice struct {
	TrackingID string
	// DevicePath is the snapshot device path in /dev.
	DevicePath string
	Major      uint32
	Minor      uint32
}

type VolumeSnapshot struct {
	// SnapshotNumber is the ID of the snapshot, as saved
	// in the CBT bitmap.
	SnapshotNumber uint32
	// GenerationID is the generation ID of this snapshot.
	GenerationID string

	// OriginalDevice is the device that was snapshot.
	OriginalDevice TrackedDevice
	// SnapshotImage is the resulting image that was created by the snapshot.
	SnapshotImage SnapshotImage
}

type SnapshotResponse struct {
	// SnapshotID is the internal ID used to delete the snapshot
	// once we are done with it.
	SnapshotID string

	// VolumeSnapshots is an array of all the disk snapshots that
	// are included in this snapshot.
	VolumeSnapshots []VolumeSnapshot
}

type DiskRange struct {
	StartOffset uint64 `json:"start_offset"`
	Length      uint64 `json:"length"`
}

type ChangesResponse struct {
	TrackedDiskID string      `json:"tracked_disk_id"`
	SnapshotID    string      `json:"snapshot_id"`
	CBTBlockSize  int         `json:"cbt_block_size_bytes"`
	BackupType    BackupType  `json:"backup_type"`
	Ranges        []DiskRange `json:"ranges"`
}
