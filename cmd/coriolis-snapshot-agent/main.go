package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"coriolis-snapshot-agent/apiserver/controllers"
	"coriolis-snapshot-agent/apiserver/routers"
	"coriolis-snapshot-agent/config"
	"coriolis-snapshot-agent/util"
	"coriolis-snapshot-agent/worker/manager"
)

var (
	conf    = flag.String("config", config.DefaultConfigFile, "exporter config file")
	version = flag.Bool("version", false, "prints version")
)

var Version string

func main() {
	flag.Parse()
	if *version {
		fmt.Println(Version)
		return
	}

	stop := make(chan os.Signal, 2)
	signal.Notify(stop, syscall.SIGTERM)
	signal.Notify(stop, syscall.SIGINT)

	cfg, err := config.ParseConfig(*conf)
	if err != nil {
		log.Fatalf("failed to parse config %s: %q", *conf, err)
	}

	logWriter, err := util.GetLoggingWriter(cfg)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(logWriter)

	ctx, cancel := context.WithCancel(context.Background())

	mgr, err := manager.NewManager(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to create manager: %q", err)
	}
	if err := mgr.Start(); err != nil {
		log.Fatalf("failed to create manager: %q", err)
	}

	// snapStorageWorker, err := worker.NewSnapStorageTracker(ctx, mgr)
	// if err != nil {
	// 	log.Fatalf("failed to create snap storage worker: %+v", err)
	// }
	// if err := snapStorageWorker.Start(); err != nil {
	// 	log.Fatalf("failed to start snap storage worker: %+v", err)
	// }
	controller, err := controllers.NewAPIController(mgr)
	if err != nil {
		log.Fatalf("failed to create controller: %+v", err)
	}

	router := routers.NewAPIRouter(controller, logWriter)

	tlsCfg, err := cfg.APIServer.TLSConfig.TLSConfig()
	if err != nil {
		log.Fatalf("failed to get TLS config: %q", err)
	}

	srv := &http.Server{
		Addr:      cfg.APIServer.BindAddress(),
		TLSConfig: tlsCfg,
		// Pass our instance of gorilla/mux in.
		Handler: router,
	}
	go func() {
		if err := srv.ListenAndServeTLS(
			cfg.APIServer.TLSConfig.Cert,
			cfg.APIServer.TLSConfig.Key); err != nil {

			log.Fatal(err)
		}
	}()

	<-stop
	cancel()
	// snapStorageWorker.Wait()
}
