package params

import (
	"coriolis-veeam-bridge/internal/types"

	"github.com/google/uuid"
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

type Volume struct {
	Path               string
	DeviceID           types.DevID
	Snapshots          int32
	LastSnapshotNumber int32
	ActiveSnapshots    []VolumeSnapshot
}

type VolumeSnapshot struct {
	// DevicePath is the snapshot device path in /dev
	DevicePath string
	// DeviceID is the major:minor number of the snapshot image
	// created in /dev.
	DeviceID types.DevID
	// SnapshotID is the internal ID used to delete the snapshot
	// once we are done with it.
	SnapshotID uuid.UUID
	// SnapshotNumber is the ID of the snapshot, as saved
	// in the CBT bitmap.
	SnapshotNumber int32
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
