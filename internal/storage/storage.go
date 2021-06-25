// Copyright 2019 Cloudbase Solutions Srl
// All Rights Reserved.
//
// This package will need a refactor after the initial implementation.
// Ideally, it should be implemented as a set of coherent interfaces, that
// may potentially be run on other system than GNU/Linux.

package storage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"

	vErrors "coriolis-veeam-bridge/errors"
)

const (
	sysfsPath     = "/sys/block"
	virtBlockPath = "/sys/devices/virtual/block"
	mountsFile    = "/proc/mounts"

	// Device types. Defined in include/scsi/scsi_proto.h
	TYPE_DISK      = 0x00
	TYPE_TAPE      = 0x01
	TYPE_PRINTER   = 0x02
	TYPE_PROCESSOR = 0x03 /* HP scanners use this */
	TYPE_WORM      = 0x04 /* Treated as ROM by our system */
	TYPE_ROM       = 0x05
	TYPE_SCANNER   = 0x06
	TYPE_MOD       = 0x07 /* Magneto-optical disk -
	* - treated as TYPE_DISK */
	TYPE_MEDIUM_CHANGER = 0x08
	TYPE_COMM           = 0x09 /* Communications device */
	TYPE_RAID           = 0x0c
	TYPE_ENCLOSURE      = 0x0d /* Enclosure Services Device */
	TYPE_RBC            = 0x0e
	TYPE_NO_LUN         = 0x7f

	// Device types. Defined in include/uapi/linux/magic.h
	ADFS_SUPER_MAGIC      = 0xadf5
	AFFS_SUPER_MAGIC      = 0xadff
	AFS_SUPER_MAGIC       = 0x5346414F
	AUTOFS_SUPER_MAGIC    = 0x0187
	CODA_SUPER_MAGIC      = 0x73757245
	CRAMFS_MAGIC          = 0x28cd3d45 /* some random number */
	CRAMFS_MAGIC_WEND     = 0x453dcd28 /* magic number with the wrong endianess */
	DEBUGFS_MAGIC         = 0x64626720
	SECURITYFS_MAGIC      = 0x73636673
	SELINUX_MAGIC         = 0xf97cff8c
	SMACK_MAGIC           = 0x43415d53 /* "SMAC" */
	RAMFS_MAGIC           = 0x858458f6 /* some random number */
	TMPFS_MAGIC           = 0x01021994
	HUGETLBFS_MAGIC       = 0x958458f6 /* some random number */
	SQUASHFS_MAGIC        = 0x73717368
	ECRYPTFS_SUPER_MAGIC  = 0xf15f
	EFS_SUPER_MAGIC       = 0x414A53
	EROFS_SUPER_MAGIC_V1  = 0xE0F5E1E2
	EXT2_SUPER_MAGIC      = 0xEF53
	EXT3_SUPER_MAGIC      = 0xEF53
	XENFS_SUPER_MAGIC     = 0xabba1974
	EXT4_SUPER_MAGIC      = 0xEF53
	BTRFS_SUPER_MAGIC     = 0x9123683E
	NILFS_SUPER_MAGIC     = 0x3434
	F2FS_SUPER_MAGIC      = 0xF2F52010
	HPFS_SUPER_MAGIC      = 0xf995e849
	ISOFS_SUPER_MAGIC     = 0x9660
	JFFS2_SUPER_MAGIC     = 0x72b6
	XFS_SUPER_MAGIC       = 0x58465342 /* "XFSB" */
	PSTOREFS_MAGIC        = 0x6165676C
	EFIVARFS_MAGIC        = 0xde5e81e4
	HOSTFS_SUPER_MAGIC    = 0x00c0ffee
	OVERLAYFS_SUPER_MAGIC = 0x794c7630
)

// Partition holds the information about a particular partition
type Partition struct {
	// Name is the name of the partition (sda1, sdb2, etc)
	Name string
	// Path is the full path for this disk.
	Path string
	// Sectors represents the size of this partitions in sectors
	// you can find the size of the partition by multiplying this
	// with the logical sector size of the disk
	Sectors int
	// FilesystemUUID represents the filesystem UUID of this partition
	FilesystemUUID string
	// PartitionUUID is the UUID of the partition. On disks with DOS partition
	// tables, the partition UUID is made up of the partition table UUID and
	// the index of the partition. This means that if the partition table has
	// am UUID of "1e21670f", then sda1 (for example) will have a partition UUID
	// of "1e21670f-01". On GPT partition tables the UUID of the partition table
	// and that of partitions are proper UUID4, and are unique.
	PartitionUUID string
	// PartitionType represents the partition type. For information about GPT
	// partition types, consult:
	// https://en.wikipedia.org/wiki/GUID_Partition_Table#Partition_type_GUIDs
	//
	// For information about MBR partition types, consult:
	// https://www.win.tue.nl/~aeb/partitions/partition_types-1.html
	PartitionType string
	// Label is the FileSystem label
	Label string
	// FilesystemType represents the name of the filesystem this
	// partition is formatted with (xfs, ext4, ntfs, etc).
	// NOTE: this may yield false positives. libblkid returns ext4
	// for the Windows Reserved partition. The FS prober returns a
	// false positive, so take this with a grain of salt.
	FilesystemType string
	// StartSector represents the sector at which the partition starts
	StartSector int
	// EndSector represents the last sector of the disk for this partition
	EndSector int
	// AlignmentOffset indicates how many bytes the beginning of the device is
	// offset from the disk's natural alignment. For details, see:
	// https://www.kernel.org/doc/Documentation/ABI/testing/sysfs-block
	AlignmentOffset int
	// Major is the device node major number
	Major uint32
	// Minor is the device minor number
	Minor uint32
	// Aliases are nodes in /dev/ that point to the same block device
	// (have the same major:minor number). These devices do not necessarily
	// have a correspondent in /sys/block
	Aliases []string
}

// BlockVolume holds information about a particular disk
type BlockVolume struct {
	// Path is the full path for this disk.
	Path string
	// PartitionTableType is the partition table type
	PartitionTableType string
	// PartitionTableUUID represents the UUID of the partition table
	PartitionTableUUID string
	// Name is just the device name, without the leading /dev
	Name string
	// Size is the size in bytes of this disk
	Size int64
	// LogicalSectorSize  is the size of the sector reported by the operating system
	// for this disk. Usually this is 512 bytes
	LogicalSectorSize int64
	// PhysicalSectorSize is the sector size reported by the disk. Some disks may have a
	// 4k sector size.
	PhysicalSectorSize int64
	// Partitions is a list of discovered partition on this disk. This is the primary
	// source of truth when identifying disks
	Partitions []Partition
	// FilesystemType represents the name of the filesystem this
	// disk is formatted with (xfs, ext4, ntfs, etc). There are situations
	// when a whole disk is formatted with a particular FS.
	// NOTE: this may yield false positives. libblkid returns ext4
	// for the Windows Reserved partition. The FS prober returns a
	// false positive, so take this with a grain of salt.
	FilesystemType string
	// AlignmentOffset indicates how many bytes the beginning of the device is
	// offset from the disk's natural alignment. For details, see:
	// https://www.kernel.org/doc/Documentation/ABI/testing/sysfs-block
	AlignmentOffset int
	// Major is the device node major number
	Major uint32
	// Minor is the device minor number
	Minor uint32
	// Aliases are nodes in /dev/ that point to the same block device
	// (have the same major:minor number). These devices do not necessarily
	// have a correspondent in /sys/block
	Aliases []string
	// DeviceMapperSlaves holds the block device(s) that back this device
	DeviceMapperSlaves []string

	// IsVirtual specifies if this device is a virtual device.
	IsVirtual bool
}

// HasMountedPartitions checks if this disk has any mounted partitions
// Normally, if we are looking at a disk we need to sync, there should be NO
// mounted partitions. Finding one means this disk is most likely a worker
// VM disk, and should be ignored.
func (b *BlockVolume) HasMountedPartitions() (bool, error) {
	mounts, err := parseMounts()
	if err != nil {
		return false, errors.Wrap(err, "parseMounts failed")
	}

	slaves, err := getDeviceMapperSlaves()
	if err != nil {
		return false, errors.Wrap(err, "getting device mapper slaves")
	}

	for _, val := range b.Partitions {
		if val.Name == "" {
			continue
		}
		devPath := path.Join("/dev", val.Name)
		if _, ok := mounts[devPath]; ok {
			return true, nil
		}

		if master, ok := slaves[val.Name]; ok {
			masterDevPath := path.Join("/dev", master)
			if _, masterOk := mounts[masterDevPath]; masterOk {
				return true, nil
			}
		}
	}
	return false, nil
}

func getPartitionInfo(pth string) (Partition, error) {
	uevent, err := parseUevent(pth)
	if err != nil {
		return Partition{}, err
	}
	partname, ok := uevent["DEVNAME"]
	if !ok || !isBlockDevice(partname) {
		return Partition{}, fmt.Errorf(
			"failed to get partition name")
	}

	start, err := getPartitionStart(pth)
	if err != nil {
		return Partition{}, err
	}

	sectors, err := getPartitionSizeInSectors(pth)
	if err != nil {
		return Partition{}, err
	}
	align, err := getAlignmentOffset(pth)
	if err != nil {
		return Partition{}, err
	}

	dev := filepath.Join("/dev", partname)
	partInfo, err := BlkIDProbe(dev)
	if err != nil {
		return Partition{}, err
	}

	dMajor, dMinor, err := GetMajorMinorFromDevice(dev)
	if err != nil {
		return Partition{}, err
	}

	fsType := partInfo["TYPE"]
	fsUUID := partInfo["UUID"]
	// Note: This only works in more recent versions of libblkid ubuntu 16.04
	// or another version of similar age should be used to get more detailed
	// information.
	partUUID := partInfo["PART_ENTRY_UUID"]
	label := partInfo["LABEL"]
	partType := partInfo["PART_ENTRY_TYPE"]
	endSector := start + sectors - 1

	return Partition{
		Path:            dev,
		Name:            partname,
		Sectors:         sectors,
		StartSector:     start,
		EndSector:       endSector,
		AlignmentOffset: align,
		PartitionType:   partType,
		FilesystemUUID:  fsUUID,
		PartitionUUID:   partUUID,
		Label:           label,
		FilesystemType:  fsType,
		Major:           dMajor,
		Minor:           dMinor,
	}, nil
}

func listDiskPartitions(pth string) ([]Partition, error) {
	partitions := []Partition{}

	info, err := os.Stat(pth)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a folder", pth)
	}

	lst, err := ioutil.ReadDir(pth)
	if err != nil {
		return nil, err
	}
	for _, val := range lst {
		if !val.IsDir() {
			continue
		}

		fullPath := path.Join(pth, val.Name())
		if _, err := os.Stat(path.Join(fullPath, "partition")); err != nil {
			continue
		}

		partInfo, err := getPartitionInfo(fullPath)
		if err != nil {
			return nil, err
		}
		partitions = append(partitions, partInfo)
	}
	return partitions, nil
}

func getBlockVolumeInfo(name string) (BlockVolume, error) {
	devicePath := path.Join("/dev", name)
	dsk, err := os.Open(devicePath)
	if err != nil {
		return BlockVolume{}, errors.Wrap(err, "could not open volume")
	}
	defer dsk.Close()

	dMajor, dMinor, err := GetMajorMinorFromDevice(devicePath)
	if err != nil {
		return BlockVolume{}, err
	}

	size, err := ioctlBlkGetSize64(dsk.Fd())
	if err != nil {
		return BlockVolume{}, errors.Wrap(err, "failed to get volume size")
	}

	physSectorSize, err := ioctlBlkPBSZGET(dsk.Fd())
	if err != nil {
		return BlockVolume{}, errors.Wrap(err, "failed to get physical sector size")
	}

	logicalSectorSize, err := ioctlBlkSSZGET(dsk.Fd())
	if err != nil {
		return BlockVolume{}, errors.Wrap(err, "failed to get logical sector size")
	}

	fullPath := path.Join(sysfsPath, name)
	align, err := getAlignmentOffset(fullPath)
	if err != nil {
		return BlockVolume{}, errors.Wrap(err, "getAllignmentOffset failed")
	}

	partitions, err := listDiskPartitions(fullPath)
	if err != nil {
		return BlockVolume{}, errors.Wrap(err, "list partitions failed")
	}

	slaves, err := getSlavesOfDevice(name)
	if err != nil {
		return BlockVolume{}, errors.Wrap(err, "list device mapper slaves")
	}

	volumeInfo, err := BlkIDProbe(devicePath)
	// Not finding any info about a disk is not necesarily an error. It may just mean
	// that the disk is raw, and was never synced.
	if err != nil && err != vErrors.ErrNoInfo {
		return BlockVolume{}, errors.Wrap(err, "blkid probe failed")
	}

	var isVirtual bool
	virtPath := path.Join(virtBlockPath, name)
	if _, err := os.Stat(virtPath); err == nil {
		isVirtual = true
	}

	// Information may be missing if this disk is raw.
	// Not finding any info is not an error at this point.
	ptType := volumeInfo["PTTYPE"]
	ptUUID := volumeInfo["PTUUID"]
	fsType := volumeInfo["TYPE"]

	vol := BlockVolume{
		AlignmentOffset:    align,
		Path:               devicePath,
		Name:               name,
		Size:               size,
		PhysicalSectorSize: physSectorSize,
		LogicalSectorSize:  logicalSectorSize,
		PartitionTableType: ptType,
		Partitions:         partitions,
		PartitionTableUUID: ptUUID,
		DeviceMapperSlaves: slaves,
		FilesystemType:     fsType,
		Major:              dMajor,
		Minor:              dMinor,
		IsVirtual:          isVirtual,
	}

	return vol, nil
}

// isValidDevice checks that the device identified by name, relative
// to /dev, is a block device, and not a loopback device or a device
// mapper mapped device.
func isValidDevice(name string) error {
	if !isBlockDevice(name) {
		return fmt.Errorf("%s not a block device", name)
	}

	if _, err := os.Stat(path.Join(sysfsPath, name)); err != nil {
		// Filter out partitions
		return fmt.Errorf("%s has no entry in %s (a partition?)", name, sysfsPath)
	}

	// Check if CD-ROM
	deviceType := path.Join(sysfsPath, name, "device", "type")
	if _, err := os.Stat(deviceType); err == nil {
		devTypeValue, err := returnContentsAsInt(deviceType)
		if err == nil {
			if devTypeValue == TYPE_ROM {
				return fmt.Errorf("%s is a CD-ROM", name)
			}
		}
	}

	return nil
}

// GetBlockDeviceInfo returns a BlockVolume{} struct with information
// about the device.
func GetBlockDeviceInfo(name string) (BlockVolume, error) {
	if err := isValidDevice(name); err != nil {
		return BlockVolume{}, vErrors.NewInvalidDeviceErr(
			"%s not a exportable block device: %s", name, err)
	}
	info, err := getBlockVolumeInfo(name)
	if err != nil {
		return BlockVolume{}, err
	}
	return info, nil
}

// BlockDeviceList returns a list of BlockVolume structures, populated with
// information about locally visible disks. This does not include the block
// device chunks.
func BlockDeviceList(ignoreMounted bool) ([]BlockVolume, error) {
	devList, err := ioutil.ReadDir(sysfsPath)
	if err != nil {
		return nil, err
	}

	ret := []BlockVolume{}
	for _, val := range devList {
		info, err := GetBlockDeviceInfo(val.Name())
		if err != nil {
			if errors.Is(err, &vErrors.ErrInvalidDevice{}) {
				continue
			}
			return ret, err
		}
		// NOTE (gsamfira): should we filter here, or before presenting the information
		// to the client? We may want to convey to the client info on mounted
		// disks as well
		// TODO (gsamfira): revisit this later
		hasMounted, err := info.HasMountedPartitions()
		if err != nil {
			return ret, errors.Wrap(err, "HasMountedPartitions failed")
		}
		if ignoreMounted && hasMounted {
			continue
		}
		ret = append(ret, info)
	}
	return ret, nil
}

// FindDeviceByID returns the path in /dev to a device identified
// by major:minor.
func FindDeviceByID(major uint32, minor uint32) (string, error) {
	devices, err := BlockDeviceList(false)
	if err != nil {
		return "", errors.Wrap(err, "fetching devices")
	}

	for _, val := range devices {
		if val.Major == major && val.Minor == minor {
			return val.Path, nil
		}

		for _, partition := range val.Partitions {
			if partition.Major == major && partition.Minor == minor {
				return partition.Path, nil
			}
		}
	}
	return "", vErrors.NewNotFoundError(
		fmt.Sprintf("could not find device [%d:%d]", major, minor))
}

// FindBlockVolumeByID returns a BlockVolume{} that identifies the device with
// major:minor.
func FindBlockVolumeByID(major uint32, minor uint32) (BlockVolume, error) {
	devices, err := BlockDeviceList(false)
	if err != nil {
		return BlockVolume{}, errors.Wrap(err, "fetching devices")
	}

	for _, val := range devices {
		if val.Major == major && val.Minor == minor {
			return val, nil
		}
	}
	return BlockVolume{}, vErrors.NewNotFoundError(
		fmt.Sprintf("could not find device [%d:%d]", major, minor))
}
