package main

import (
	"coriolis-veeam-bridge/config"
	"coriolis-veeam-bridge/internal/ioctl"
	"flag"
	"fmt"
)

var (
	conf = flag.String("config", config.DefaultConfigFile, "exporter config file")
)

func main() {
	// flag.Parse()

	// stop := make(chan os.Signal, 2)
	// signal.Notify(stop, syscall.SIGTERM)
	// signal.Notify(stop, syscall.SIGINT)

	// cfg, err := config.ParseConfig(*conf)
	// if err != nil {
	// 	log.Fatalf("failed to parse config %s: %q", *conf, err)
	// }

	// logWriter, err := util.GetLoggingWriter(cfg)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// log.SetOutput(logWriter)

	// controller, err := controllers.NewAPIController(cfg)
	// if err != nil {
	// 	log.Fatalf("failed to create controller: %q", err)
	// }

	// router := routers.NewAPIRouter(controller, logWriter)

	// tlsCfg, err := cfg.APIServer.TLSConfig.TLSConfig()
	// if err != nil {
	// 	log.Fatalf("failed to get TLS config: %q", err)
	// }

	// srv := &http.Server{
	// 	Addr:      cfg.APIServer.BindAddress(),
	// 	TLSConfig: tlsCfg,
	// 	// Pass our instance of gorilla/mux in.
	// 	Handler: router,
	// }
	// go func() {
	// 	if err := srv.ListenAndServeTLS(
	// 		cfg.APIServer.TLSConfig.Cert,
	// 		cfg.APIServer.TLSConfig.Key); err != nil {

	// 		log.Fatal(err)
	// 	}
	// }()

	// <-stop

	// params := types.DevID{
	// 	Major: 252,
	// 	Minor: 0,
	// }
	// params2 := types.DevID{
	// 	Major: 252,
	// 	Minor: 32,
	// }
	// snapDevice := types.DevID{
	// 	Major: 252,
	// 	Minor: 17,
	// }

	// snap_file := "/mnt/snapstores/veeam_file"
	// // snap_file2 := "/mnt/snapstores/veeam_file_vdc"

	// snapStore, err := ioctl.CreateSnapStore([]types.DevID{params}, snapDevice)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// snapStore2, err := ioctl.CreateSnapStore([]types.DevID{params}, snapDevice)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Println(snapStore2)
	// if err := ioctl.SnapStoreAddFile(snapStore, snap_file); err != nil {
	// 	log.Fatal(err)
	// }
	// if err := ioctl.SnapStoreAddFile(snapStore2, snap_file2); err != nil {
	// 	log.Fatal(err)
	// }

	// if err := ioctl.AddDeviceToTracking(params2); err != nil {
	// 	log.Fatal(err)
	// }

	// if err := ioctl.AddDeviceToTracking(params); err != nil {
	// 	log.Fatal(err)
	// }

	// cbtInfo, err := ioctl.GetCBTInfo()
	info, err := ioctl.GetCBTInfo()
	if err != nil {
		fmt.Printf("%+v\n", err)
		return
	}

	fmt.Println(info)

	// js, _ := json.MarshalIndent(cbtInfo, "", "  ")
	// fmt.Println(string(js))

	// snapshot, err := ioctl.CreateSnapshot([]types.DevID{params, params2})
	// if err != nil {
	// 	fmt.Printf(">>> %+v\n", err)
	// 	return
	// }

	// bitmap, err := ioctl.GetCBTBitmap(params)

	// if err != nil {
	// 	fmt.Printf("%+v\n", err)
	// 	return
	// }

	// for idx, val := range bitmap.Buff {
	// 	if val != 0 {
	// 		fmt.Printf("sector nr %d changed in snapshot %d\n", idx, val)
	// 	}
	// }
	// fmt.Println(snapshot.SnapshotID)
	// fmt.Println(snapshot.Count)

	// time.Sleep(30 * time.Second)
	// if err := ioctl.DeleteSnapshot(snapshot.SnapshotID); err != nil {
	// 	fmt.Printf(">>> %+v\n", err)
	// 	return
	// }

	// cleanUp, err := ioctl.SnapStoreCleanup(snapStore)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// fmt.Println(cleanUp)

	// devs, err := storage.BlockDeviceList(false)
	// if err != nil {
	// 	fmt.Printf("%+v\n", err)
	// 	return
	// }
	// js, _ := json.MarshalIndent(devs, "", "  ")
	// fmt.Println(string(js))

	// // pre-allocate space on a device to hold the snap store data.
	// snap_file := "/mnt/snapstores/veeam_file"
	// // err = internal.CreateSnapStoreFile(snap_file, 2048*1024*1024)
	// // fmt.Println(err)

	// //r1, r2, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_ADD, uintptr(unsafe.Pointer(&params)))
	// //fmt.Println(r1, r2, err)

	// //r1, r2, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_REMOVE, uintptr(unsafe.Pointer(&params)))
	// //fmt.Println(r1, r2, err)

	// //r1, r2, err := syscall.Syscall(syscall.SYS_IOCTL, dev.Fd(), IOCTL_TRACKING_COLLECT, uintptr(unsafe.Pointer(&trParams)))
	// //fmt.Println(r1, r2, err, info)
}
