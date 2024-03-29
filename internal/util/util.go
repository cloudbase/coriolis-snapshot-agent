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

package util

import (
	"fmt"
	"os"
	"syscall"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"coriolis-snapshot-agent/apiserver/params"
	veeamErrors "coriolis-snapshot-agent/errors"
	"coriolis-snapshot-agent/internal/storage"
	"coriolis-snapshot-agent/internal/types"
)

type PhysicalDiskInfo struct {
	Major      uint32
	Minor      uint32
	DevicePath string
	SectorSize int64
}

type FileSystemInfo struct {
	// Type is the type of filesystem
	Type int64
	// BlockSize is the optimal transfer block size
	BlockSize int64
	// Blocks is the total data blocks in filesystem
	Blocks uint64
	// BlocksFree is the number of free blocks in filesystem
	BlocksFree uint64
	// BlocksAvailable is the total number of free blocks
	// available to unprivileged user
	BlocksAvailable uint64

	// BytesFree is the free amount of disk space in bytes.
	BytesFree uint64
}

// FindAllInvolvedDevices accepts an array of device ids, and determins whether or
// not they are part of a device mapper. If they are, all involved devices will be
// returned as an array of device IDs.
// We currently cannot safely allocate extents meant for CoW pages on a device mapper
// due to the fact that Coriolis needs to synd disk data of raw physical disks, not
// of individual partitions.
func FindAllInvolvedDevices(devices []types.DevID) ([]string, error) {
	var ret []string
	allDevices, err := storage.BlockDeviceList(false, true, true)
	if err != nil {
		return nil, errors.Wrap(err, "fetching devices")
	}

	devicePaths := map[string]types.DevID{}
	for _, val := range devices {
		devPath, err := storage.FindDeviceByID(val.Major, val.Minor)
		if err != nil {
			return nil, errors.Wrap(err, "finding device path")
		}
		devicePaths[devPath] = val
	}

	for _, val := range allDevices {
		found := false
		if _, ok := devicePaths[val.Path]; ok {
			ret = append(ret, val.Path)
			found = true
		} else {
			for _, part := range val.Partitions {
				if _, ok := devicePaths[part.Path]; ok {
					ret = append(ret, val.Path)
					found = true
				}
				break
			}
		}

		if found && val.IsVirtual {
			ret = append(ret, val.DeviceMapperSlaves...)
		}
	}
	return ret, nil
}

// getBlockDeviceInfoFromFile returns info about the block device that hosts the
// file.
func GetBlockDeviceInfoFromFile(path string) (PhysicalDiskInfo, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return PhysicalDiskInfo{}, errors.Wrap(err, "running Stat()")
	}
	sysStat := fileInfo.Sys().(*syscall.Stat_t)
	// For a file, the Rdev is not relevant. The device that is returned here
	// may be a device mapper, which can point to multiple block devices
	// (LVM, RAID, etc).
	major := unix.Major(sysStat.Dev)
	minor := unix.Minor(sysStat.Dev)

	devices, err := storage.BlockDeviceList(false, true, true)
	if err != nil {
		return PhysicalDiskInfo{}, errors.Wrap(err, "fetching block devices")
	}
	for _, val := range devices {
		if val.Major == major && val.Minor == minor {
			return PhysicalDiskInfo{
				Major:      val.Major,
				Minor:      val.Minor,
				SectorSize: val.LogicalSectorSize,
				DevicePath: val.Path,
			}, nil
		}

		for _, part := range val.Partitions {
			if part.Major == major && part.Minor == minor {
				return PhysicalDiskInfo{
					Major:      part.Major,
					Minor:      part.Minor,
					SectorSize: val.LogicalSectorSize,
					DevicePath: part.Path,
				}, nil
			}
		}
	}
	return PhysicalDiskInfo{}, veeamErrors.NewNotFoundError(
		fmt.Sprintf("could not find device for file %s", fileInfo.Name()))
}

func GetFileSystemInfoFromPath(path string) (FileSystemInfo, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return FileSystemInfo{}, errors.Wrap(err, "fetching filesystem information")
	}

	return FileSystemInfo{
		Type:            stat.Type,
		BlockSize:       stat.Bsize,
		Blocks:          stat.Blocks,
		BlocksFree:      stat.Bfree,
		BlocksAvailable: stat.Bavail,
		BytesFree:       uint64(stat.Bsize * int64(stat.Bfree)),
	}, nil
}

// CreateSnapStoreFile creates a new pre-allocated file of the given size.
func CreateSnapStoreFile(filePath string, size uint64) error {
	// TODO: Return ranges, range count and device major:minor

	if _, err := os.Stat(filePath); err == nil {
		return errors.Errorf("file already exists")
	}

	fd, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "creating file")
	}

	fallocFlags := []uint32{
		unix.FALLOC_FL_ZERO_RANGE,
	}
	var fallocErr error

	for i := 0; i < len(fallocFlags); i++ {
		if err := syscall.Fallocate(int(fd.Fd()), fallocFlags[0], 0, int64(size)); err != nil {
			fallocErr = err
			continue
		}
		if err := fd.Close(); err != nil {
			return errors.Wrap(err, "closing file")
		}

		return nil
	}
	// Fallocate not supported?
	// returns only the last error
	return errors.Wrap(fallocErr, "running fallocate")
}

func GetFileRanges(filePath string) ([]types.Range, types.DevID, error) {
	bDevInfo, err := GetBlockDeviceInfoFromFile(filePath)
	if err != nil {
		return nil, types.DevID{}, errors.Wrap(err, "fetching block device info")
	}
	extents, err := GetExtents(filePath)
	if err != nil {
		return nil, types.DevID{}, errors.Wrap(err, "fetching extents")
	}

	var ret []types.Range
	for _, val := range extents {
		ret = append(ret, types.Range{
			Left:  val.Physical,
			Right: (val.Physical + val.Length - 1),
		})
	}

	return ret, types.DevID{
		Major: bDevInfo.Major,
		Minor: bDevInfo.Minor,
	}, nil
}

func FindDeviceByPath(path string) (types.DevID, error) {
	devices, err := storage.BlockDeviceList(false, true, true)
	if err != nil {
		return types.DevID{}, errors.Wrap(err, "fetching block devices")
	}

	for _, val := range devices {
		if val.Path == path {
			return types.DevID{
				Major: val.Major,
				Minor: val.Minor,
			}, nil
		}

		for _, part := range val.Partitions {
			if part.Path == path {
				return types.DevID{
					Major: part.Major,
					Minor: part.Minor,
				}, nil
			}
		}
	}
	return types.DevID{}, errors.Errorf("device %s not found", path)
}

// InternalBlockVolumeToParamsBlockVolume converts an internal block volume
// to a params block volume. We copy the values because we want to be free to
// change de underlying storage implementation in the future, without changing
// the response the consumers get. I realize this is a premature abstraction,
// but I could not bring myself to add json tags to a structure inside the
// internal package, let alone return it as is to the API consumer.
func InternalBlockVolumeToParamsBlockVolume(volume storage.BlockVolume) params.BlockVolume {
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
		ret.Partitions = append(ret.Partitions, InternalPartitionToParamsPartition(val))
	}
	return ret
}

func InternalPartitionToParamsPartition(partition storage.Partition) params.Partition {
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
