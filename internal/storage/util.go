// Copyright 2019 Cloudbase Solutions Srl
// All Rights Reserved.

package storage

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

const (
	// BLKGETSIZE64 is the ioctl needed to get the block count
	// of a particular disk
	BLKGETSIZE64 = 0x80081272

	// BLKSSZGET is the ioctl needed to get the logical sector
	// size of a disk
	BLKSSZGET = 0x1268
	// BLKPBSZGET is the ioctl needed to get the physical sector
	// size
	BLKPBSZGET = 0x127b
)

// parseMounts returns a map of block device to mountpoint. Any device mapper
// links will be resolved to the actual dm-X block device. This will be helpful
// later when we need to determine if a block device is a slave to a block device.
func parseMounts() (map[string]string, error) {
	file, err := os.Open(mountsFile)
	if err != nil {
		return map[string]string{}, errors.Wrap(err, "open mounts failed")
	}
	defer file.Close()

	ret := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.Split(scanner.Text(), " ")
		if len(line) < 2 {
			continue
		}

		if info, err := os.Lstat(line[0]); err == nil {
			mode := info.Mode()
			if mode&os.ModeSymlink != 0 {
				pth, err := filepath.EvalSymlinks(line[0])
				if err != nil {
					return ret, errors.Wrap(err, "eval symling failed")
				}
				ret[pth] = line[1]
			} else {
				ret[line[0]] = line[1]
			}
		}
	}
	return ret, nil
}

// getDeviceMapperSlaves returns a map of block devices that points
// to a device mapper to which they are slaves. We need this information
// to reliably identify raw block devices which are in use.
func getDeviceMapperSlaves() (map[string]string, error) {
	// partition --> mapped device
	ret := map[string]string{}

	devList, err := ioutil.ReadDir(sysfsPath)
	if err != nil {
		return ret, err
	}

	for _, master := range devList {
		fullPath := path.Join(sysfsPath, master.Name())
		slavesDir := path.Join(fullPath, "slaves")
		if _, err := os.Stat(slavesDir); err != nil {
			continue
		}

		slaves, err := ioutil.ReadDir(slavesDir)
		if err != nil {
			continue
		}
		for _, slave := range slaves {
			ret[slave.Name()] = master.Name()
		}
	}
	return ret, nil
}

func getSlavesOfDevice(name string) ([]string, error) {
	var ret []string
	devPath := path.Join(sysfsPath, name)
	if _, err := os.Stat(devPath); err != nil {
		return nil, fmt.Errorf("device %s does not exist", name)
	}

	slavesDir := path.Join(devPath, "slaves")
	slaves, err := ioutil.ReadDir(slavesDir)
	if err != nil {
		return ret, nil
	}
	for _, slave := range slaves {
		ret = append(ret, path.Join("/dev", slave.Name()))
	}
	return ret, nil
}

// ioctlBlkGetSize64 returns the size of the block device
func ioctlBlkGetSize64(fd uintptr) (int64, error) {
	var size int64
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, BLKGETSIZE64, uintptr(unsafe.Pointer(&size))); err != 0 {
		return 0, err
	}
	return size, nil
}

// ioctlBlkPBSZGET returns the physical sector size for a disk
func ioctlBlkPBSZGET(fd uintptr) (int64, error) {
	var size int64
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, BLKPBSZGET, uintptr(unsafe.Pointer(&size))); err != 0 {
		return 0, err
	}
	return size, nil
}

// ioctlBlkSSZGET returns the logical sector size for a disk
func ioctlBlkSSZGET(fd uintptr) (int64, error) {
	var size int64
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, BLKSSZGET, uintptr(unsafe.Pointer(&size))); err != 0 {
		return 0, err
	}
	return size, nil
}

// isBlockDevice returns true if a device is a block device.
// This includes loop devices.
func isBlockDevice(name string) bool {
	devPath := path.Join("/dev", name)
	info, err := os.Stat(devPath)
	if err != nil {
		return false
	}
	mode := info.Mode()
	if mode&os.ModeDevice == os.ModeDevice && mode&os.ModeCharDevice != os.ModeCharDevice {
		return true
	}

	return false
}

// returnContentsAsInt parses a file and casts the contents of the file as an int
// This is only useful if that file has only one integer inside it. This is currently
// being used to parse some files in /sys.
func returnContentsAsInt(path string) (int, error) {
	if _, err := os.Stat(path); err != nil {
		return 0, err
	}
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, err
	}
	if len(contents) == 0 {
		return 0, fmt.Errorf("failed to read %s", path)
	}
	asInt, err := strconv.Atoi(string(contents[:len(contents)-1]))
	if err != nil {
		return 0, err
	}
	return asInt, nil
}

func getPartitionStart(pth string) (int, error) {
	startFile := path.Join(pth, "start")
	return returnContentsAsInt(startFile)
}

func getPartitionSizeInSectors(pth string) (int, error) {
	sizeFile := path.Join(pth, "size")
	return returnContentsAsInt(sizeFile)
}

func getAlignmentOffset(pth string) (int, error) {
	sizeFile := path.Join(pth, "alignment_offset")
	return returnContentsAsInt(sizeFile)
}

func GetMajorMinorFromDevice(devicePath string) (uint32, uint32, error) {
	st, err := stat(devicePath)
	if err != nil {
		return 0, 0, err
	}

	return unix.Major(st.Rdev), unix.Minor(st.Rdev), nil
}

func parseUevent(pth string) (map[string]string, error) {
	uevent := path.Join(pth, "uevent")
	if _, err := os.Stat(uevent); err != nil {
		return map[string]string{}, err
	}

	ret := map[string]string{}

	file, err := os.Open(uevent)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.SplitN(scanner.Text(), "=", 2)
		ret[line[0]] = line[1]
	}

	return ret, nil
}
func stat(path string) (syscall.Stat_t, error) {
	stat := syscall.Stat_t{}
	if err := syscall.Stat(path, &stat); err != nil {
		return syscall.Stat_t{}, err
	}
	return stat, nil
}
