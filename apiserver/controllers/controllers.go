package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"coriolis-veeam-bridge/apiserver/params"
	"coriolis-veeam-bridge/config"
	gErrors "coriolis-veeam-bridge/errors"
	"coriolis-veeam-bridge/manager"
)

// NewAPIController returns a new instance of APIController
func NewAPIController(cfg *config.Config) (*APIController, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "validating config")
	}

	mgr, err := manager.NewManager(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "opening database")
	}
	return &APIController{
		cfg: cfg,
		mgr: mgr,
	}, nil
}

func parseBoolParam(arg string, defaultValue bool) bool {
	if arg == "" {
		return defaultValue
	}
	parsed, _ := strconv.ParseBool(arg)
	return parsed
}

func handleError(w http.ResponseWriter, err error) {
	w.Header().Add("Content-Type", "application/json")
	origErr := errors.Cause(err)
	apiErr := params.APIErrorResponse{
		Details: origErr.Error(),
	}

	switch origErr.(type) {
	case *gErrors.NotFoundError:
		w.WriteHeader(http.StatusNotFound)
		apiErr.Error = "Not Found"
	case *gErrors.UnauthorizedError:
		w.WriteHeader(http.StatusUnauthorized)
		apiErr.Error = "Not Authorized"
	case *gErrors.BadRequestError:
		w.WriteHeader(http.StatusBadRequest)
		apiErr.Error = "Bad Request"
	case *gErrors.ConflictError:
		w.WriteHeader(http.StatusConflict)
		apiErr.Error = "Conflict"
	default:
		log.Printf("Unhandled error: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		apiErr.Error = "Server error"
	}

	json.NewEncoder(w).Encode(apiErr)
}

// APIController implements all API handlers.
type APIController struct {
	cfg *config.Config
	mgr *manager.Snapshot
}

// ListVMsHandler lists all VMs from all repositories on the system.
func (a *APIController) ListDisksHandler(w http.ResponseWriter, r *http.Request) {
	includeVirtualArg := r.URL.Query().Get("includeVirtual")
	includeVirtual := parseBoolParam(includeVirtualArg, false)
	disks, err := a.mgr.ListDisks(includeVirtual)
	if err != nil {
		log.Printf("failed to list disks: %q", err)
		handleError(w, err)
		return
	}
	json.NewEncoder(w).Encode(disks)
}

func (a *APIController) GetDiskHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	diskID, ok := vars["diskTrackingID"]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	disk, err := a.mgr.GetTrackedDisk(diskID)
	if err != nil {
		log.Printf("failed to get disk: %q", err)
		fmt.Printf("%+v\n", err)
		handleError(w, err)
		return
	}
	json.NewEncoder(w).Encode(disk)
}

func (a *APIController) ListSnapStoreLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := a.mgr.ListAvailableSnapStoreLocations()
	if err != nil {
		log.Printf("failed to list virtual machines: %q", err)
		handleError(w, err)
		return
	}
	json.NewEncoder(w).Encode(locations)
}

// // GetVMHandler gets information about a single VM.
// func (a *APIController) GetVMHandler(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	vmID, ok := vars["vmID"]
// 	if !ok {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}
// 	vmInfo, err := a.mgr.GetVirtualMachine(vmID)
// 	if err != nil {
// 		log.Printf("failed to get virtual machines: %q", err)
// 		handleError(w, err)
// 		return
// 	}
// 	json.NewEncoder(w).Encode(vmInfo)
// }

// // ListSnapshotsHandler lists all snapshots for a VM.
// func (a *APIController) ListSnapshotsHandler(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	vmID, ok := vars["vmID"]
// 	if !ok {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}

// 	snaps, err := a.mgr.ListSnapshots(vmID)
// 	if err != nil {
// 		log.Printf("failed to list snapshots: %q", err)
// 		handleError(w, err)
// 		return
// 	}
// 	json.NewEncoder(w).Encode(snaps)
// }

// // GetSnapshotHandler gets information about a single snapshot for a VM. It takes an optional
// // query arg diff, which allows comparison of current snapshot, with a previous snapshot.
// // The snapshot we are comparing to must exist and must be older than the current one.
// func (a *APIController) GetSnapshotHandler(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	vmID, ok := vars["vmID"]
// 	if !ok {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}
// 	snapID, ok := vars["snapshotID"]
// 	if !ok {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}

// 	squashChunksParam := r.URL.Query().Get("squashChunks")
// 	var squashChunks bool
// 	if squashChunksParam == "" {
// 		// Default to true
// 		squashChunks = true
// 	} else {
// 		squashChunks, _ = strconv.ParseBool(squashChunksParam)
// 	}

// 	compareTo := r.URL.Query().Get("compareTo")
// 	snapshot, err := a.mgr.GetSnapshot(vmID, snapID, compareTo, squashChunks)
// 	if err != nil {
// 		log.Printf("failed to get snapshot: %q", err)
// 		handleError(w, err)
// 		return
// 	}
// 	json.NewEncoder(w).Encode(snapshot)
// }

// // DeleteSnapshotHandler removes one snapshot associated with a VM.
// func (a *APIController) DeleteSnapshotHandler(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	vmID, ok := vars["vmID"]
// 	if !ok {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}
// 	snapID, ok := vars["snapshotID"]
// 	if !ok {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}

// 	err := a.mgr.DeleteSnapshot(vmID, snapID)
// 	if err != nil {
// 		log.Printf("failed to delete snapshot: %q", err)
// 		handleError(w, err)
// 		return
// 	}
// 	w.WriteHeader(http.StatusOK)
// }

// // PurgeSnapshotsHandler deletes all snapshots associated with a VM.
// func (a *APIController) PurgeSnapshotsHandler(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	vmID, ok := vars["vmID"]
// 	if !ok {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}
// 	if err := a.mgr.PurgeSnapshots(vmID); err != nil {
// 		log.Printf("failed to purge snapshots: %q", err)
// 		handleError(w, err)
// 	}
// 	w.WriteHeader(http.StatusOK)
// }

// // CreateSnapshotHandler creates a snapshots for a VM.
// func (a *APIController) CreateSnapshotHandler(w http.ResponseWriter, r *http.Request) {
// 	// CreateSnapshot
// 	vars := mux.Vars(r)
// 	vmID, ok := vars["vmID"]
// 	if !ok {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}

// 	snapData, err := a.mgr.CreateSnapshot(vmID)
// 	if err != nil {
// 		log.Printf("failed to create snapshot: %q", err)
// 		handleError(w, err)
// 		return
// 	}
// 	json.NewEncoder(w).Encode(snapData)
// }

// // ConsumeSnapshotHandler allows the caller to download arbitrary ranges of disk data from a
// // disk snapshot.
// func (a *APIController) ConsumeSnapshotHandler(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	vmID, ok := vars["vmID"]
// 	if !ok {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}
// 	snapID, ok := vars["snapshotID"]
// 	if !ok {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}

// 	diskID, ok := vars["diskID"]
// 	if !ok {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}

// 	snapshot, err := a.mgr.GetSnapshot(vmID, snapID, "", false)
// 	if err != nil {
// 		log.Printf("failed to get snapshot: %q", err)
// 		handleError(w, err)
// 		return
// 	}

// 	var disk params.DiskSnapshot
// 	for _, val := range snapshot.Disks {
// 		if val.Name == diskID {
// 			disk = val
// 		}
// 	}

// 	if disk.Name == "" {
// 		w.WriteHeader(http.StatusNotFound)
// 		return
// 	}

// 	fp, err := os.Open(disk.Path)
// 	if err != nil {
// 		log.Printf("failed open snapshot file: %q", err)
// 		w.WriteHeader(http.StatusInternalServerError)
// 		return
// 	}
// 	defer fp.Close()
// 	http.ServeContent(w, r, disk.Path, time.Time{}, fp)
// }

// NotFoundHandler is returned when an invalid URL is acccessed
func (a *APIController) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	apiErr := params.APIErrorResponse{
		Details: "Resource not found",
		Error:   "Not found",
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(apiErr)
}
