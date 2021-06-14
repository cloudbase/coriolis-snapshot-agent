package main

import (
	"coriolis-veeam-bridge/internal/ioctl"
	"fmt"
	"log"
)

//	"time"
//	"github.com/google/uuid"

func main() {

	params := ioctl.DevID{
		Major: 252,
		Minor: 0,
	}

	snapStore, err := ioctl.CreateSnapStore(params, ioctl.DevID{})
	if err != nil {
		log.Fatal(err)
	}

	if err := ioctl.SnapStoreAddMemory(snapStore, 2048*1024*1024); err != nil {
		log.Fatal(err)
	}

	snapshot, err := ioctl.CreateSnapshot(params)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(snapshot.SnapshotID)

	if err := ioctl.DeleteSnapshot(snapshot.SnapshotID); err != nil {
		log.Fatal(err)
	}

	cleanUp, err := ioctl.SnapStoreCleanup(snapStore)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(cleanUp)
	// devs, err := storage.BlockDeviceList(false)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// js, _ := json.MarshalIndent(devs, "", "  ")
	// fmt.Println(string(js))

	// // Snap Cleanup
	// parsedUUID, err := uuid.Parse("e72a0232-7219-4dbe-8732-cc0b963b3ae5")
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }

	// snapCleanup := internal.SnapStoreCleanup{
	// 	ID: [16]byte(parsedUUID),
	// }
	// r1, _, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), internal.IOCTL_SNAPSTORE_CLEANUP, uintptr(unsafe.Pointer(&snapCleanup)))
	// fmt.Println(r1, err, snapCleanup)
	// if r1 != 0 {
	// 	fmt.Printf("Error creating store: %v --> %v\n", r1, err.Error())
	// 	return
	// }

	// // pre-allocate space on a device to hold the snap store data.
	// snap_file := "/mnt/snapstores/veeam_file"
	// // err = internal.CreateSnapStoreFile(snap_file, 2048*1024*1024)
	// // fmt.Println(err)

	// ranges, devID, err := internal.GetFileRanges(snap_file)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }

	// SnapStoreFileAdd
	// ranges is a slice of extents, where we pre-allocated disk space to
	// hold the snap store data. There is no way to predict how many extents
	// we'll have after we pre-allocate the space, this will always be a slice.
	//
	// When adding a new file as snap store storage, we need to pass in a fixed
	// sized array. The extra header information that a slice has, will result in
	// errors when calling IOCTL. We need to get the underlying array that the
	// slice points to, and pass that to IOCTL. This is not safe. Getting an uintptr
	// will not prevent the memory address to move between the time when we cast it
	// to uintptr and the time the syscall is run.

	// hdr := (*reflect.SliceHeader)(unsafe.Pointer(&ranges))
	// fRangesUintPtr := hdr.Data

	// storeFile := internal.SnapStoreFileAdd{
	// 	ID:         uuidAsBytes,
	// 	RangeCount: uint32(len(ranges)),
	// 	Ranges:     uintptrToByte(fRangesUintPtr),
	// }
	// r1, _, err = syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), internal.IOCTL_SNAPSTORE_FILE, uintptr(unsafe.Pointer(&storeFile)))
	// fmt.Println(r1, err, storeFile)
	// if r1 != 0 {
	// 	fmt.Printf("Error adding file: %v --> %v\n", r1, err.Error())
	// 	return
	// }

	// cbtMap := make([]byte, cbtInfo.cbt_map_size)

	// readCBT := ioctl.TrackingReadCBTBitmap{
	// 	DevID:  params,
	// 	Length: uint32(cbtInfo.cbt_map_size),
	// 	Buff:   &cbtMap[0], //uintptrToByte(uintptr(unsafe.Pointer(&cbtMap[0]))),
	// }
	// r1, _, err = syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), ioctl.IOCTL_TRACKING_READ_CBT_BITMAP, uintptr(unsafe.Pointer(&readCBT)))
	// fmt.Println(r1, err, readCBT)

	// for idx, val := range cbtMap {
	// 	if val != 0 {
	// 		fmt.Printf("Sector at offset %d was changed in snapshot %d\n", 512*idx, val)
	// 	}
	// }

	// info := internal.CBTInfo{
	// 	DevID: params,
	// }

	// trParams := internal.TrackingCollect{
	// 	Count:   1,
	// 	CBTInfo: uintptrToByte(uintptr(unsafe.Pointer(&info))),
	// }
	// r1, _, err = syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), internal.IOCTL_TRACKING_COLLECT, uintptr(unsafe.Pointer(&trParams)))
	// fmt.Println(r1, err, info)
	// if r1 != 0 {
	// 	fmt.Printf("Error setting store limit: %d --> %q\n", r1, err.Error())
	// 	return
	// }

	// id := uuid.UUID(info.GenerationID)
	// fmt.Println(id.String())

	// snapshot create
	// snapParams := internal.SnapshotCreate{
	// 	SnapshotID: 0,
	// 	Count:      1,
	// 	DevID:      uintptrToByte(uintptr(unsafe.Pointer(&params))),
	// }
	// r1, _, err = syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), internal.IOCTL_SNAPSHOT_CREATE, uintptr(unsafe.Pointer(&snapParams)))
	// fmt.Println(r1, err, snapParams)
	// if r1 != 0 {
	// 	fmt.Printf("Error setting store limit: %d --> %q\n", r1, err.Error())
	// 	return
	// }

	// //r1, r2, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_ADD, uintptr(unsafe.Pointer(&params)))
	// //fmt.Println(r1, r2, err)

	// //r1, r2, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_REMOVE, uintptr(unsafe.Pointer(&params)))
	// //fmt.Println(r1, r2, err)

	// //r1, r2, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_COLLECT, uintptr(unsafe.Pointer(&trParams)))
	// //fmt.Println(r1, r2, err, info)
}
