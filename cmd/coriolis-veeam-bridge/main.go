package main

import (
	"encoding/json"
	"fmt"

	"coriolis-veeam-bridge/internal/ioctl"
	"coriolis-veeam-bridge/internal/types"
	"log"
)

//	"time"
//	"github.com/google/uuid"

func main() {

	params := types.DevID{
		Major: 252,
		Minor: 0,
	}
	// snapDevice := types.DevID{
	// 	Major: 252,
	// 	Minor: 17,
	// }

	// snap_file := "/mnt/snapstores/veeam_file"

	// snapStore, err := ioctl.CreateSnapStore(params, snapDevice)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// if err := ioctl.SnapStoreAddFile(snapStore, snap_file); err != nil {
	// 	log.Fatal(err)
	// }

	cbtInfo, err := ioctl.GetCBTInfo(params)
	if err != nil {
		log.Fatal(err)
	}

	js, _ := json.MarshalIndent(cbtInfo, "", "  ")
	fmt.Println(string(js))

	snapshot, err := ioctl.CreateSnapshot(params)
	if err != nil {
		log.Fatal(err)
	}

	bitmap, err := ioctl.GetCBTBitmap(params)

	if err != nil {
		fmt.Printf("%+v\n", err)
		return
	}

	for idx, val := range bitmap.Buff {
		if val != 0 {
			fmt.Printf("sector nr %d changed in snapshot %d\n", idx, val)
		}
	}
	// fmt.Println(snapshot.SnapshotID)
	// fmt.Println(snapshot.Count)

	if err := ioctl.DeleteSnapshot(snapshot.SnapshotID); err != nil {
		log.Fatal(err)
	}

	// cleanUp, err := ioctl.SnapStoreCleanup(snapStore)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// fmt.Println(cleanUp)

	// devs, err := storage.BlockDeviceList(false)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// js, _ := json.MarshalIndent(devs, "", "  ")
	// fmt.Println(string(js))

	// // pre-allocate space on a device to hold the snap store data.
	// snap_file := "/mnt/snapstores/veeam_file"
	// // err = internal.CreateSnapStoreFile(snap_file, 2048*1024*1024)
	// // fmt.Println(err)

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

	// //r1, r2, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_ADD, uintptr(unsafe.Pointer(&params)))
	// //fmt.Println(r1, r2, err)

	// //r1, r2, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_REMOVE, uintptr(unsafe.Pointer(&params)))
	// //fmt.Println(r1, r2, err)

	// //r1, r2, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_COLLECT, uintptr(unsafe.Pointer(&trParams)))
	// //fmt.Println(r1, r2, err, info)
}
