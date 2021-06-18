package ioctl

/*
#cgo CFLAGS: -I/usr/src/veeamsnap-5.0.0.0
#include "veeamsnap_ioctl.h"
#include <string.h>
#include <stdlib.h>

void setCBTInfo(struct ioctl_tracking_collect_s* collect, struct cbt_info_s* cbtInfo) {
	collect->p_cbt_info = cbtInfo;
}

void setSnapshotImages(struct ioctl_collect_shapshot_images_s* images, struct image_info_s* imgInfo) {
	images->p_image_info = imgInfo;
}

void setSnapStoreDev(struct ioctl_snapstore_create_s* snapStore, struct ioctl_dev_id_s* devID) {
	snapStore->p_dev_id = devID;
}

void setSnapshotDevice(struct ioctl_snapshot_create_s* snapshot, struct ioctl_dev_id_s* devID) {
	snapshot->p_dev_id = devID;
}

void setSnapStoreFileRanges(struct ioctl_snapstore_file_add_s* fileAdd, struct ioctl_range_s* ranges) {
	fileAdd->ranges = ranges;
}

void setSnapStoreFileMultiDevRanges(struct ioctl_snapstore_file_add_multidev_s* fileAdd, struct ioctl_range_s* ranges) {
	fileAdd->ranges = ranges;
}

void setCBTBitmapBuffer(struct ioctl_tracking_read_cbt_bitmap_s* cbtBitmapParams, unsigned char* buff) {
	cbtBitmapParams->buff = buff;
}

int get_values(struct cbt_info_s *vals, int idx, unsigned int size, struct cbt_info_s *converted) {
	if (idx > size-1) {
		return -1;
	}
	converted->dev_id = vals[idx].dev_id;
	converted->dev_capacity = vals[idx].dev_capacity;
	converted->cbt_map_size = vals[idx].cbt_map_size;
	converted->snap_number = vals[idx].snap_number;
	memcpy(converted->generationId, vals[idx].generationId, 16);

	return 0;
}
*/
import "C"

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"

	"coriolis-veeam-bridge/internal/types"
	"coriolis-veeam-bridge/internal/util"
)

func cToGoUUID(uuid [16]C.uchar) (goUUID [16]byte) {
	for idx, val := range uuid {
		goUUID[idx] = byte(val)
	}
	return
}

func goToCUUID(uuid [16]byte) (cUUID [16]C.uchar) {
	for idx, val := range uuid {
		cUUID[idx] = C.uchar(val)
	}
	return
}

// Tracking

// GetCBTInfo returns Change Block Tracking information for one or multiple devices.
// Change Block Tracking creates a bitmap equal in size with the number of sectors that
// compose the device you add under tracking. The value of each element in the bitmap
// will be equal to the snapshot number in which that sector was last changed.
// The bitmap itself is capable of keeping track of 2^8 snapshots, before it is reset,
// and a full sync/backup will have to be executed again. To keep track of which version
// of the bitmap your snapshot coresponds to, you will need to use the GenerationID field
// of the types.CBTInfo struct. IF what you saved in your application does not match the
// GenerationID returned here, you *must* do a full sync.
func GetCBTInfo() ([]types.CBTInfo, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	// Get tracked disks count
	trackingCollectCount := C.struct_ioctl_tracking_collect_s{
		count: C.uint(0),
	}

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_COLLECT, uintptr(unsafe.Pointer(&trackingCollectCount)))
	if r1 != 0 {
		return nil, errors.Wrap(err, "running ioctl")
	}

	size := C.ulong(C.sizeof_struct_cbt_info_s * trackingCollectCount.count)
	buffer := C.malloc(size)
	defer C.free(unsafe.Pointer(buffer))

	var cbtInfo *C.struct_cbt_info_s = (*C.struct_cbt_info_s)(buffer)

	trackingCollect := C.struct_ioctl_tracking_collect_s{
		count: trackingCollectCount.count,
	}
	C.setCBTInfo(&trackingCollect, cbtInfo)

	r1, _, err = syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_COLLECT, uintptr(unsafe.Pointer(&trackingCollect)))
	if r1 != 0 {
		return nil, errors.Wrap(err, "running ioctl")
	}

	var cbtInfoRet []types.CBTInfo

	for i := 0; i < int(trackingCollectCount.count); i++ {
		var val C.struct_cbt_info_s
		if ret := C.get_values(cbtInfo, C.int(i), trackingCollectCount.count, &val); ret != 0 {
			return nil, fmt.Errorf("failed to fetch")
		}

		var generationID [16]byte
		data := C.GoBytes(unsafe.Pointer(&val.generationId[0]), C.int(16))
		copy(generationID[:], data)

		cbtInfoRet = append(cbtInfoRet, types.CBTInfo{
			DevID: types.DevID{
				Major: uint32(val.dev_id.major),
				Minor: uint32(val.dev_id.minor),
			},
			DevCapacity:  uint64(val.dev_capacity),
			CBTMapSize:   uint32(val.cbt_map_size),
			SnapNumber:   byte(val.snap_number),
			GenerationID: generationID,
		})
	}

	return cbtInfoRet, nil
}

func AddDeviceToTracking(device types.DevID) error {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()
	deviceParams := C.struct_ioctl_dev_id_s{
		major: C.int(device.Major),
		minor: C.int(device.Minor),
	}

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_ADD, uintptr(unsafe.Pointer(&deviceParams)))
	if r1 != 0 {
		return errors.Wrap(err, "running ioctl")
	}

	return nil
}

func RemoveDeviceFromTracking(device types.DevID) error {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()
	deviceParams := C.struct_ioctl_dev_id_s{
		major: C.int(device.Major),
		minor: C.int(device.Minor),
	}

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_REMOVE, uintptr(unsafe.Pointer(&deviceParams)))
	if r1 != 0 {
		return errors.Wrap(err, "running ioctl")
	}

	return nil
}

func GetTrackingBlockSize() (uint32, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return 0, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	var blkSize uint32
	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_BLOCK_SIZE, uintptr(unsafe.Pointer(&blkSize)))
	if r1 != 0 {
		return 0, errors.Wrap(err, "running ioctl")
	}
	return blkSize, nil
}

func GetCBTBitmap(device types.DevID) (types.TrackingReadCBTBitmap, error) {
	cbtInfo, err := GetCBTInfo()
	if err != nil {
		return types.TrackingReadCBTBitmap{}, errors.Wrap(err, "fetching bitmap")
	}

	var deviceCBTInfo types.CBTInfo
	for _, val := range cbtInfo {
		if val.DevID.Major == device.Major && val.DevID.Minor == device.Minor {
			deviceCBTInfo = val
			break
		}
	}

	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return types.TrackingReadCBTBitmap{}, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	buff := make([]C.uchar, deviceCBTInfo.CBTMapSize)

	cbtBitmapParams := C.struct_ioctl_tracking_read_cbt_bitmap_s{
		dev_id: C.struct_ioctl_dev_id_s{
			major: C.int(device.Major),
			minor: C.int(device.Minor),
		},
		length: C.uint(deviceCBTInfo.CBTMapSize),
	}
	C.setCBTBitmapBuffer(&cbtBitmapParams, &buff[0])

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_READ_CBT_BITMAP, uintptr(unsafe.Pointer(&cbtBitmapParams)))
	if uint32(r1) != deviceCBTInfo.CBTMapSize {
		return types.TrackingReadCBTBitmap{}, errors.Wrap(err, "running ioctl")
	}

	goBuff := make([]byte, deviceCBTInfo.CBTMapSize)

	for idx, val := range buff {
		goBuff[idx] = byte(val)
	}

	return types.TrackingReadCBTBitmap{
		DevID:  device,
		Offset: uint32(cbtBitmapParams.offset),
		Length: uint32(cbtBitmapParams.length),
		Buff:   goBuff,
	}, nil
}

// Snap store

// CreateSnapStore creates a new snap store. There are 3 types of snap stores:
//   * Memory - snap store data is hel in memory. To create a memory snap store, set the
// Major and Minor numbers for snapDevice to 0. Extents will be added using the
// IOCTL_SNAPSTORE_MEMORY ioctl call.
//   * Single device - A snap store is expected to resize on a single volume. Extents will
// be allocated via falloc, or other methods, on this device alone, and will be added to the
// snap store. To create a single device snap store, specify the correct Major and Minor
// numbers of the device you want to use. Extents will then be added using the IOCTL_SNAPSTORE_FILE
// ioctl call.
//   * Multi device - A snap store is expected to reside on multiple devices. To create a multi dev
// snap store, set the Major and Minor numbers of snapDevice to -1. Extents will be added using the
// IOCTL_SNAPSTORE_FILE_MULTIDEV ioctl call. The parameters for this call, includes the device ID of
// the volume and the extens on this device that will be allocated to the snap store.
func CreateSnapStore(storeID [16]byte, device []types.DevID, snapDevice types.DevID) (types.SnapStore, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return types.SnapStore{}, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	// newUUID := uuid.New()
	// uuidAsBytes := [16]byte(newUUID)

	devID := make([]C.struct_ioctl_dev_id_s, len(device))
	for idx, val := range device {
		devID[idx] = C.struct_ioctl_dev_id_s{
			major: C.int(val.Major),
			minor: C.int(val.Minor),
		}
	}
	snapStore := C.struct_ioctl_snapstore_create_s{
		id:    goToCUUID(storeID),
		count: 1,
	}
	if snapDevice.Major != 0 && snapDevice.Minor != 0 {
		snapStore.snapstore_dev_id = C.struct_ioctl_dev_id_s{
			major: C.int(snapDevice.Major),
			minor: C.int(snapDevice.Minor),
		}
	}
	C.setSnapStoreDev(&snapStore, &devID[0])

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_SNAPSTORE_CREATE, uintptr(unsafe.Pointer(&snapStore)))
	if r1 != 0 {
		return types.SnapStore{}, errors.Wrap(err, "running ioctl")
	}

	return types.SnapStore{
		ID:               cToGoUUID(snapStore.id),
		SnapshotDeviceID: snapDevice,
		Count:            1,
		DevID:            device,
	}, nil
}

func SnapStoreAddMemory(snapStore types.SnapStore, size uint64) error {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	memLimit := C.struct_ioctl_snapstore_memory_limit_s{
		id:   goToCUUID(snapStore.ID),
		size: C.ulonglong(size),
	}

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_SNAPSTORE_MEMORY, uintptr(unsafe.Pointer(&memLimit)))
	fmt.Println(r1, err, memLimit)
	if r1 != 0 {
		return errors.Wrap(err, "running ioctl")
	}
	return nil
}

func SnapStoreAddFile(snapStore types.SnapStore, file string) error {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	ranges, devID, err := util.GetFileRanges(file)
	if err != nil {
		if err != nil {
			return errors.Wrap(err, "fetching file ranges")
		}
	}
	if snapStore.SnapshotDeviceID.Major != devID.Major && snapStore.SnapshotDeviceID.Minor != devID.Minor {
		return fmt.Errorf("snap store file is on different device than snapshot device")
	}

	cRanges := make([]C.struct_ioctl_range_s, len(ranges))
	for idx, val := range ranges {
		cRanges[idx] = C.struct_ioctl_range_s{
			left:  C.ulonglong(val.Left),
			right: C.ulonglong(val.Right),
		}
	}
	snapAddFile := C.struct_ioctl_snapstore_file_add_s{
		id:          goToCUUID(snapStore.ID),
		range_count: C.uint(len(ranges)),
	}
	C.setSnapStoreFileRanges(&snapAddFile, &cRanges[0])
	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_SNAPSTORE_FILE, uintptr(unsafe.Pointer(&snapAddFile)))
	if r1 != 0 {
		return errors.Wrap(err, "running ioctl")
	}
	return nil
}

func SnapStoreAddFileMultiDev(snapStore types.SnapStore, file string) error {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	ranges, devID, err := util.GetFileRanges(file)
	if err != nil {
		if err != nil {
			return errors.Wrap(err, "fetching file ranges")
		}
	}

	cRanges := make([]C.struct_ioctl_range_s, len(ranges))
	snapAddFile := C.struct_ioctl_snapstore_file_add_multidev_s{
		id:          goToCUUID(snapStore.ID),
		range_count: C.uint(len(ranges)),
		dev_id: C.struct_ioctl_dev_id_s{
			major: C.int(devID.Major),
			minor: C.int(devID.Minor),
		},
	}
	C.setSnapStoreFileMultiDevRanges(&snapAddFile, &cRanges[0])
	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_SNAPSTORE_FILE_MULTIDEV, uintptr(unsafe.Pointer(&snapAddFile)))
	if r1 != 0 {
		return errors.Wrap(err, "running ioctl")
	}
	return nil
}

func SnapStoreCleanup(snapStore types.SnapStore) (types.SnapStoreCleanupParams, error) {
	snapCleanup := C.struct_ioctl_snapstore_cleanup_s{
		id: goToCUUID(snapStore.ID),
	}

	return types.SnapStoreCleanupParams{
		ID:          snapStore.ID,
		FilledBytes: uint64(snapCleanup.filled_bytes),
	}, nil
}

// Snapshot
// CreateSnapshot creates a single snapshot of one or more devices. When snapshotting multiple
// devices, you must make sure that you either create one snapstore per disk, each with its own
// storage, or you create a multi device snap store and add a number of files equal to the nr
// of disks you are snapshotting.
func CreateSnapshot(device []types.DevID) (types.Snapshot, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return types.Snapshot{}, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	devID := make([]C.struct_ioctl_dev_id_s, len(device))
	for idx, val := range device {
		devID[idx] = C.struct_ioctl_dev_id_s{
			major: C.int(val.Major),
			minor: C.int(val.Minor),
		}
	}

	snapshotParams := C.struct_ioctl_snapshot_create_s{
		count: C.uint(len(device)),
	}
	C.setSnapshotDevice(&snapshotParams, &devID[0])

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_SNAPSHOT_CREATE, uintptr(unsafe.Pointer(&snapshotParams)))
	if r1 != 0 {
		return types.Snapshot{}, errors.Wrap(err, "running ioctl")
	}
	return types.Snapshot{
		SnapshotID: uint64(snapshotParams.snapshot_id),
		Count:      uint32(snapshotParams.count),
		DevID:      device,
	}, nil
}

// DeleteSnapshot deletes a single snapshot identified by snapshotID. A single snapshot
// may be of one or of multiple devices.
func DeleteSnapshot(snapshotID uint64) error {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_SNAPSHOT_DESTROY, uintptr(unsafe.Pointer(&snapshotID)))
	if r1 != 0 {
		return errors.Wrap(err, "running ioctl")
	}
	return nil
}

// Snapshot images
func CollectSnapshotImages() (types.SnapshotImages, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return types.SnapshotImages{}, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	// Get total number of images
	snapImgsCount := C.struct_ioctl_collect_shapshot_images_s{
		count: 0,
	}
	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_COLLECT_SNAPSHOT_IMAGES, uintptr(unsafe.Pointer(&snapImgsCount)))
	if r1 != 0 && err != syscall.ENODATA {
		// ENODATA is to be expected, as we are only fetching the number of
		// images. Anything else, should be treated as an error.
		return types.SnapshotImages{}, errors.Wrap(err, "running ioctl")
	}

	if snapImgsCount.count == 0 {
		// No images available.
		return types.SnapshotImages{}, nil
	}

	// Fetch image info.
	infoArr := make([]C.struct_image_info_s, snapImgsCount.count)
	snapImgs := C.struct_ioctl_collect_shapshot_images_s{
		count: snapImgsCount.count,
	}
	C.setSnapshotImages(&snapImgs, &infoArr[0])

	r1, _, err = syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_COLLECT_SNAPSHOT_IMAGES, uintptr(unsafe.Pointer(&snapImgs)))
	if r1 != 0 {
		return types.SnapshotImages{}, errors.Wrap(err, "running ioctl")
	}

	convertedImgInfo := []types.ImageInfo{}
	for _, val := range infoArr {
		convertedImgInfo = append(convertedImgInfo, types.ImageInfo{
			OriginalDevID: types.DevID{
				Major: uint32(val.original_dev_id.major),
				Minor: uint32(val.original_dev_id.minor),
			},
			SnapshotDevID: types.DevID{
				Major: uint32(val.snapshot_dev_id.major),
				Minor: uint32(val.snapshot_dev_id.minor),
			},
		})
	}

	return types.SnapshotImages{
		Count:     uint32(len(convertedImgInfo)),
		ImageInfo: convertedImgInfo,
	}, nil
}
