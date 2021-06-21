package controllers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"coriolis-veeam-bridge/apiserver/params"
	vErrors "coriolis-veeam-bridge/errors"
	"coriolis-veeam-bridge/manager"
)

// NewAPIController returns a new instance of APIController
func NewAPIController(mgr *manager.Snapshot) (*APIController, error) {
	return &APIController{
		mgr: mgr,
	}, nil
}

// APIController implements all API handlers.
type APIController struct {
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

func (a *APIController) ListSnapStoreHandler(w http.ResponseWriter, r *http.Request) {
	snapStores, err := a.mgr.ListSnapStores()
	if err != nil {
		log.Printf("failed to get disk: %+v", err)
		handleError(w, err)
		return
	}
	json.NewEncoder(w).Encode(snapStores)
}

// Snapshots
func (a *APIController) CreateSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	var newSnapshot params.CreateSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&newSnapshot); err != nil {
		handleError(w, vErrors.ErrBadRequest)
		return
	}

	err := a.mgr.CreateSnapshot(newSnapshot)
	if err != nil {
		log.Printf("failed to get disk: %+v", err)
		handleError(w, err)
		return
	}
	json.NewEncoder(w).Encode(newSnapshot)
}

func (a *APIController) ListSnapshotsHandler(w http.ResponseWriter, r *http.Request) {
}

func (a *APIController) GetSnapshotHandler(w http.ResponseWriter, r *http.Request) {
}

func (a *APIController) DeleteSnapshotHandler(w http.ResponseWriter, r *http.Request) {
}

// Snap store mappings

func (a *APIController) CreateSnapStoreMappingHandler(w http.ResponseWriter, r *http.Request) {
	// CreateSnapStore
	var newSnapData params.CreateSnapStoreMappingRequest
	if err := json.NewDecoder(r.Body).Decode(&newSnapData); err != nil {
		handleError(w, vErrors.ErrBadRequest)
		return
	}

	if newSnapData.SnapStoreLocation == "" || newSnapData.TrackedDisk == "" {
		handleError(w, vErrors.ErrBadRequest)
		return
	}

	response, err := a.mgr.CreateSnapStoreMapping(newSnapData)
	if err != nil {
		log.Printf("failed to get disk: %+v", err)
		handleError(w, err)
		return
	}
	json.NewEncoder(w).Encode(response)
}

func (a *APIController) ListSnapStoreMappingsHandler(w http.ResponseWriter, r *http.Request) {
	snapStores, err := a.mgr.ListSnapStoreMappings()
	if err != nil {
		log.Printf("failed to get disk: %+v", err)
		handleError(w, err)
		return
	}
	json.NewEncoder(w).Encode(snapStores)
}

// utils
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
	case *vErrors.NotFoundError:
		w.WriteHeader(http.StatusNotFound)
		apiErr.Error = "Not Found"
	case *vErrors.UnauthorizedError:
		w.WriteHeader(http.StatusUnauthorized)
		apiErr.Error = "Not Authorized"
	case *vErrors.BadRequestError:
		w.WriteHeader(http.StatusBadRequest)
		apiErr.Error = "Bad Request"
	case *vErrors.ConflictError:
		w.WriteHeader(http.StatusConflict)
		apiErr.Error = "Conflict"
	default:
		log.Printf("Unhandled error: %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		apiErr.Error = "Server error"
	}

	json.NewEncoder(w).Encode(apiErr)
}

// NotFoundHandler is returned when an invalid URL is acccessed
func (a *APIController) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	apiErr := params.APIErrorResponse{
		Details: "Resource not found",
		Error:   "Not found",
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(apiErr)
}
