package util

import (
	"fmt"
	"os"
	"syscall"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	veeamErrors "coriolis-veeam-bridge/errors"
	"coriolis-veeam-bridge/internal/storage"
	"coriolis-veeam-bridge/internal/types"
)

type PhysicalDiskInfo struct {
	Major      uint32
	Minor      uint32
	DevicePath string
	SectorSize int64
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

	devices, err := storage.BlockDeviceList(false)
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

// CreateSnapStoreFile creates a new pre-allocated file of the given size.
func CreateSnapStoreFile(filePath string, size int64) error {
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
		if err := syscall.Fallocate(int(fd.Fd()), fallocFlags[0], 0, size); err != nil {
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
	devices, err := storage.BlockDeviceList(false)
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
