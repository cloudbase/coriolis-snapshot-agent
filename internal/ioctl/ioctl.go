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
*/
import "C"

import (
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

func GetCBTInfo(device DevID) (CBTInfo, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return CBTInfo{}, errors.Wrap(err, "opening veeamsnap")
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
		return CBTInfo{}, errors.Wrap(err, "running ioctl")
	}

	var generationID [16]byte
	data := C.GoBytes(unsafe.Pointer(&cbtInfo.generationId[0]), C.int(16))
	copy(generationID[:], data)

	return CBTInfo{
		DevID:        device,
		DevCapacity:  uint64(cbtInfo.dev_capacity),
		CBTMapSize:   uint32(cbtInfo.cbt_map_size),
		SnapNumber:   byte(cbtInfo.snap_number),
		GenerationID: generationID,
	}, nil
}

func CollectSnapshotImages(device DevID) (SnapshotImages, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return SnapshotImages{}, errors.Wrap(err, "opening veeamsnap")
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
		return SnapshotImages{}, errors.Wrap(err, "running ioctl")
	}

	convertedImgInfo := []ImageInfo{}
	for _, val := range infoArr {
		if uint32(val.snapshot_dev_id.major) == 0 && uint32(val.snapshot_dev_id.minor) == 0 {
			break
		}
		convertedImgInfo = append(convertedImgInfo, ImageInfo{
			OriginalDevID: device,
			SnapshotDevID: DevID{
				Major: uint32(val.snapshot_dev_id.major),
				Minor: uint32(val.snapshot_dev_id.minor),
			},
		})
	}

	return SnapshotImages{
		Count:     uint32(len(convertedImgInfo)),
		ImageInfo: convertedImgInfo,
	}, nil
}

func CreateSnapStore(device DevID, snapDevice DevID) (SnapStore, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return SnapStore{}, errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	newUUID := uuid.New()
	uuidAsBytes := [16]byte(newUUID)
	cCharUUID := [16]C.uchar{}
	for idx, val := range uuidAsBytes {
		cCharUUID[idx] = C.uchar(val)
	}

	devID := C.struct_ioctl_dev_id_s{
		major: C.int(device.Major),
		minor: C.int(device.Minor),
	}
	snapStore := C.struct_ioctl_snapstore_create_s{
		id:    cCharUUID,
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
		return SnapStore{}, errors.Wrap(err, "running ioctl")
	}

	// may differ from one we requested
	actualUUID := [16]byte{}
	for idx, val := range snapStore.id {
		actualUUID[idx] = byte(val)
	}
	return SnapStore{
		ID:               actualUUID,
		SnapshotDeviceID: snapDevice,
		Count:            1,
		DevID:            &device,
	}, nil
}

func SnapStoreAddMemory(snapStore SnapStore, size uint64) error {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return errors.Wrap(err, "opening veeamsnap")
	}
	defer dev.Close()

	cCharUUID := [16]C.uchar{}
	for idx, val := range snapStore.ID {
		cCharUUID[idx] = C.uchar(val)
	}

	memLimit := C.struct_ioctl_snapstore_memory_limit_s{
		id:   cCharUUID,
		size: C.ulonglong(size),
	}

	r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_SNAPSTORE_MEMORY, uintptr(unsafe.Pointer(&memLimit)))
	fmt.Println(r1, err, memLimit)
	if r1 != 0 {
		return errors.Wrap(err, "running ioctl")
	}
	return nil
}

func SnapStoreCleanup(snapStore SnapStore) (SnapStoreCleanupParams, error) {
	snapID := [16]C.uchar{}
	for idx, val := range snapStore.ID {
		snapID[idx] = C.uchar(val)
	}
	snapCleanup := C.struct_ioctl_snapstore_cleanup_s{
		id: snapID,
	}

	return SnapStoreCleanupParams{
		ID:          snapStore.ID,
		FilledBytes: uint64(snapCleanup.filled_bytes),
	}, nil
}

func CreateSnapshot(device DevID) (Snapshot, error) {
	dev, err := os.OpenFile(VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return Snapshot{}, errors.Wrap(err, "opening veeamsnap")
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
		return Snapshot{}, errors.Wrap(err, "running ioctl")
	}
	return Snapshot{
		SnapshotID: uint64(snapshotParams.snapshot_id),
		Count:      1,
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
