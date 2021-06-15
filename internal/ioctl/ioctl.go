package ioctl

/*
#cgo CFLAGS: -I/usr/src/veeamsnap-5.0.0.0
#include "veeamsnap_ioctl.h"

void setCBTInfo(struct cbt_info_s* cbtInfo, struct ioctl_tracking_collect_s* collect) {
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

void setCBTBitmapBuffer(struct ioctl_tracking_read_cbt_bitmap_s* cbtBitmapParams, unsigned char* buff) {
	cbtBitmapParams->buff = buff;
}
*/
import "C"

import (
	"coriolis-veeam-bridge/internal/types"
	"coriolis-veeam-bridge/internal/util"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const (
	MaxSnapshotImages = 255
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
func GetCBTInfo(device types.DevID) (types.CBTInfo, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return types.CBTInfo{}, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	cbtInfo := C.struct_cbt_info_s{
		dev_id: C.struct_ioctl_dev_id_s{
			major: C.int(device.Major),
			minor: C.int(device.Minor),
		},
	}
	trackingCollect := C.struct_ioctl_tracking_collect_s{
		count: 1,
	}
	C.setCBTInfo(&cbtInfo, &trackingCollect)

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_COLLECT, uintptr(unsafe.Pointer(&trackingCollect)))
	if r1 != 0 {
		return types.CBTInfo{}, errors.Wrap(err, "running ioctl")
	}

	var generationID [16]byte
	data := C.GoBytes(unsafe.Pointer(&cbtInfo.generationId[0]), C.int(16))
	copy(generationID[:], data)

	return types.CBTInfo{
		DevID:        device,
		DevCapacity:  uint64(cbtInfo.dev_capacity),
		CBTMapSize:   uint32(cbtInfo.cbt_map_size),
		SnapNumber:   byte(cbtInfo.snap_number),
		GenerationID: generationID,
	}, nil
}

func GetTrackingBlockSize() (blkSize uint32, err error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return 0, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_BLOCK_SIZE, uintptr(unsafe.Pointer(&blkSize)))
	if r1 != 0 {
		return 0, errors.Wrap(err, "running ioctl")
	}
	return blkSize, nil
}

func GetCBTBitmap(device types.DevID) (types.TrackingReadCBTBitmap, error) {
	cbtInfo, err := GetCBTInfo(device)
	if err != nil {
		return types.TrackingReadCBTBitmap{}, errors.Wrap(err, "fetching bitmap")
	}

	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return types.TrackingReadCBTBitmap{}, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	buff := make([]C.uchar, cbtInfo.CBTMapSize)

	cbtBitmapParams := C.struct_ioctl_tracking_read_cbt_bitmap_s{
		dev_id: C.struct_ioctl_dev_id_s{
			major: C.int(device.Major),
			minor: C.int(device.Minor),
		},
		length: C.uint(cbtInfo.CBTMapSize),
	}
	C.setCBTBitmapBuffer(&cbtBitmapParams, &buff[0])

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_READ_CBT_BITMAP, uintptr(unsafe.Pointer(&cbtBitmapParams)))
	if uint32(r1) != cbtInfo.CBTMapSize {
		return types.TrackingReadCBTBitmap{}, errors.Wrap(err, "running ioctl")
	}

	goBuff := make([]byte, cbtInfo.CBTMapSize)

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
func CreateSnapStore(device types.DevID, snapDevice types.DevID) (types.SnapStore, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return types.SnapStore{}, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	newUUID := uuid.New()
	uuidAsBytes := [16]byte(newUUID)

	devID := C.struct_ioctl_dev_id_s{
		major: C.int(device.Major),
		minor: C.int(device.Minor),
	}
	snapStore := C.struct_ioctl_snapstore_create_s{
		id:    goToCUUID(uuidAsBytes),
		count: 1,
	}
	if snapDevice.Major != 0 && snapDevice.Minor != 0 {
		snapStore.snapstore_dev_id = C.struct_ioctl_dev_id_s{
			major: C.int(snapDevice.Major),
			minor: C.int(snapDevice.Minor),
		}
	}
	C.setSnapStoreDev(&snapStore, &devID)

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_SNAPSTORE_CREATE, uintptr(unsafe.Pointer(&snapStore)))
	if r1 != 0 {
		return types.SnapStore{}, errors.Wrap(err, "running ioctl")
	}

	return types.SnapStore{
		ID:               cToGoUUID(snapStore.id),
		SnapshotDeviceID: snapDevice,
		Count:            1,
		DevID:            &device,
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
func CreateSnapshot(device types.DevID) (types.Snapshot, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return types.Snapshot{}, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	devID := C.struct_ioctl_dev_id_s{
		major: C.int(device.Major),
		minor: C.int(device.Minor),
	}
	snapshotParams := C.struct_ioctl_snapshot_create_s{
		count: 1,
	}
	C.setSnapshotDevice(&snapshotParams, &devID)

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_SNAPSHOT_CREATE, uintptr(unsafe.Pointer(&snapshotParams)))
	if r1 != 0 {
		return types.Snapshot{}, errors.Wrap(err, "running ioctl")
	}
	return types.Snapshot{
		SnapshotID: uint64(snapshotParams.snapshot_id),
		Count:      uint32(snapshotParams.count),
		DevID:      &device,
	}, nil
}

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
func CollectSnapshotImages(device types.DevID) (types.SnapshotImages, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return types.SnapshotImages{}, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	infoArr := [MaxSnapshotImages]C.struct_image_info_s{}
	for i := 0; i < MaxSnapshotImages; i++ {
		infoArr[i] = C.struct_image_info_s{
			original_dev_id: C.struct_ioctl_dev_id_s{
				major: C.int(device.Major),
				minor: C.int(device.Minor),
			},
		}
	}

	snapImgs := C.struct_ioctl_collect_shapshot_images_s{
		count: MaxSnapshotImages,
	}
	C.setSnapshotImages(&snapImgs, &infoArr[0])

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_COLLECT_SNAPSHOT_IMAGES, uintptr(unsafe.Pointer(&snapImgs)))
	if r1 != 0 {
		return types.SnapshotImages{}, errors.Wrap(err, "running ioctl")
	}

	convertedImgInfo := []types.ImageInfo{}
	for _, val := range infoArr {
		if uint32(val.snapshot_dev_id.major) == 0 && uint32(val.snapshot_dev_id.minor) == 0 {
			break
		}
		convertedImgInfo = append(convertedImgInfo, types.ImageInfo{
			OriginalDevID: device,
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
