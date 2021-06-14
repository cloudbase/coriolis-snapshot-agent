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
	"strings"

	"github.com/pkg/errors"

	veeamErrors "coriolis-veeam-bridge/errors"
)

const (
	sysfsPath     = "/sys/block"
	virtBlockPath = "/sys/devices/virtual/block"
	mountsFile    = "/proc/mounts"
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

	dMajor, dMinor, err := getMajorMinorFromDevice(dev)
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

	dMajor, dMinor, err := getMajorMinorFromDevice(devicePath)
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
	if err != nil && err != veeamErrors.ErrNoInfo {
		return BlockVolume{}, errors.Wrap(err, "blkid probe failed")
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
	}

	return vol, nil
}

func isDeviceMapper(name string) bool {
	hintPath := path.Join(virtBlockPath, name, "dm")
	if _, err := os.Stat(hintPath); err == nil {
		return true
	}
	return false
}

// isValidDevice checks that the device identified by name, relative
// to /dev, is a block device, and not a loopback device or a device
// mapper mapped device.
func isValidDevice(name string) error {
	if !isBlockDevice(name) {
		return fmt.Errorf("%s not a block device", name)
	}

	if strings.HasPrefix(name, "loop") {
		return fmt.Errorf("%s is a loop device", name)
	}

	if _, err := os.Stat(path.Join(sysfsPath, name)); err != nil {
		// Filter out partitions
		return fmt.Errorf("%s has no entry in %s (a partition?)", name, sysfsPath)
	}

	virtPath := path.Join(virtBlockPath, name)
	if _, err := os.Stat(virtPath); err == nil {
		// We want device mappers
		if !isDeviceMapper(name) {
			// exclude loop, ram, etc.
			return fmt.Errorf("%s is a virtual device", name)
		}
	}

	if removable, err := returnContentsAsInt(path.Join(sysfsPath, name, "removable")); err == nil {
		if removable == 1 {
			return fmt.Errorf("%s is removable", name)
		}
	}

	return nil
}

// GetBlockDeviceInfo returns a BlockVolume{} struct with information
// about the device.
func GetBlockDeviceInfo(name string) (BlockVolume, error) {
	if err := isValidDevice(name); err != nil {
		return BlockVolume{}, veeamErrors.NewInvalidDeviceErr(
			fmt.Sprintf("%s not a exportable block device: %s", name, err))
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
	devList, err := ioutil.ReadDir("/dev")
	if err != nil {
		return nil, err
	}

	ret := []BlockVolume{}
	for _, val := range devList {
		info, err := GetBlockDeviceInfo(val.Name())
		if err != nil {
			if veeamErrors.IsInvalidDevice(err) {
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
