package manager

import (
	"coriolis-veeam-bridge/apiserver/params"
	"coriolis-veeam-bridge/db"
	vErrors "coriolis-veeam-bridge/errors"
	"coriolis-veeam-bridge/internal/ioctl"
	"coriolis-veeam-bridge/internal/types"
	"coriolis-veeam-bridge/internal/util"
	"coriolis-veeam-bridge/worker/common"
	"coriolis-veeam-bridge/worker/snapstore"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/timshannon/bolthold"
)

/////////////////
// Snap stores //
/////////////////

func (m *Snapshot) snapStoreUsedBytes(snapStoreID string) (uint64, error) {
	m.mux.Lock()
	defer m.mux.Unlock()

	uuidParsed, err := uuid.Parse(snapStoreID)
	if err != nil {
		return 0, errors.Wrap(err, "parsing UUID")
	}
	snapStoreParams := types.SnapStore{
		ID: [16]byte(uuidParsed),
	}

	snapStore, err := m.db.GetSnapStore(snapStoreID)
	if err != nil {
		return 0, errors.Wrap(err, "getting snap store")
	}

	snapshots, err := m.db.ListSnapshotsForDisk(snapStore.TrackedDisk.TrackingID)
	if err != nil {
		return 0, errors.Wrap(err, "listing snapshots for disk")
	}

	if len(snapshots) == 0 {
		// Calling ioctl.SnapStoreCleanup on an empty store, will delete the snap store.
		// When there is at least one snapshot, ioctl.SnapStoreCleanup will return the
		// amount of disk space ocupied by the CoW data. Return 0 and nil here, because
		// there is no space ocupied by a snap store with no snapshots associated.
		return 0, nil
	}

	// log.Printf("fetching info about %s", snapStoreID)
	snapStoreRet, err := ioctl.SnapStoreCleanup(snapStoreParams)
	if err != nil {
		return 0, errors.Wrap(err, "fetching snap store usage")
	}
	if snapStoreRet.FilledBytes == ioctl.SNAP_STORE_NOT_FOUND {
		return 0, vErrors.NewNotFoundError("snap store %s does not exist", snapStoreID)
	}
	// log.Printf("Filled bytes for %s is %d", snapStoreID, snapStoreRet.FilledBytes)
	return snapStoreRet.FilledBytes, nil
}

// CreateSnapStore creates a new snap store via ioctl.
func (m *Snapshot) CreateSnapStore(trackedDisk string) (db.SnapStore, error) {
	var err error
	log.Printf("creating snap store for disk %s", trackedDisk)
	_, err = m.db.FindSnapStoresForDevice(trackedDisk)
	if err != nil {
		if !errors.Is(err, bolthold.ErrNotFound) {
			return db.SnapStore{}, errors.Wrap(err, "fetching snap store")
		}
	} else {
		return db.SnapStore{}, vErrors.NewConflictError("device %s already has a snap store", trackedDisk)
	}

	mapping, err := m.db.GetSnapStoreMappingByDeviceID(trackedDisk)
	if err != nil {
		return db.SnapStore{}, errors.Wrap(err, "fetching mapping")
	}

	snapStoreLocation, err := m.db.GetSnapStoreFilesLocationByID(mapping.SnapStoreFilesLocation.Path)
	if err != nil {
		return db.SnapStore{}, errors.Wrap(err, "fetching snap store location")
	}

	disk, err := m.db.GetTrackedDiskByTrackingID(trackedDisk)
	if err != nil {
		if errors.Is(err, bolthold.ErrNotFound) {
			return db.SnapStore{}, vErrors.NewNotFoundError("no such tracked disk: %s", trackedDisk)
		}
		return db.SnapStore{}, errors.Wrap(err, "fetching tracked disk")
	}

	deviceID := types.DevID{
		Major: disk.Major,
		Minor: disk.Minor,
	}

	log.Printf("tracked disk ID is %d:%d", disk.Major, disk.Minor)
	snapDisk := types.DevID{
		Major: snapStoreLocation.Major,
		Minor: snapStoreLocation.Minor,
	}

	newUUID := uuid.New()
	uuidAsBytes := [16]byte(newUUID)

	newSnapStoreParams := db.SnapStore{
		SnapStoreID:     newUUID.String(),
		TrackedDisk:     disk,
		StorageLocation: snapStoreLocation,
	}

	store, err := m.db.CreateSnapStore(newSnapStoreParams)
	if err != nil {
		return db.SnapStore{}, errors.Wrap(err, "adding store to db")
	}

	defer func() {
		if err != nil {
			m.db.DeleteSnapStore(store.SnapStoreID)
		}
	}()

	if store.Path() == "" {
		return db.SnapStore{}, errors.Errorf("snap store is invalid")
	}

	log.Printf("Checking for snap store location folder: %s", store.Path())
	if _, statErr := os.Stat(store.Path()); statErr != nil {
		if !errors.Is(statErr, fs.ErrNotExist) {
			return db.SnapStore{}, errors.Wrap(statErr, "checking storage location")
		}
		log.Printf("Creating snap store location folder: %s", store.Path())

		if mkdirErr := os.MkdirAll(store.Path(), 00770); mkdirErr != nil {
			log.Printf("Error creating snap store location folder %s: %q", store.Path(), mkdirErr)
			return db.SnapStore{}, errors.Wrap(mkdirErr, "creating storage location")
		}

		defer func(storageNeedsInit bool) {
			if err != nil && storageNeedsInit {
				log.Printf("Cleaning snap store location folder %s due to error: %q", store.Path(), err)
				os.RemoveAll(store.Path())
			}
		}(true)
	}

	snapCharacterDeviceWatcherParams := common.CreateSnapStoreParams{
		ID:                uuidAsBytes,
		BaseDir:           store.Path(),
		SnapDeviceID:      snapDisk,
		DeviceID:          deviceID,
		SnapStoreFileSize: m.cfg.SnapStoreFileSize,
	}
	snapCharacterDeviceWatcher, err := snapstore.NewSnapStoreCharacterDeviceWatcher(snapCharacterDeviceWatcherParams, m.msgChan)
	if err != nil {
		return db.SnapStore{}, errors.Wrap(err, "creating snap store")
	}
	m.RecordWatcher(newUUID.String(), snapCharacterDeviceWatcher)
	return store, nil
}

func (m *Snapshot) ListSnapStores() ([]params.SnapStoreResponse, error) {
	stores, err := m.db.ListSnapStores()
	if err != nil {
		return nil, errors.Wrap(err, "fetching snap stores")
	}
	if len(stores) == 0 {
		return []params.SnapStoreResponse{}, nil
	}
	resp := make([]params.SnapStoreResponse, len(stores))

	for idx, val := range stores {
		files, err := m.db.FindSnapStoreFiles(val.SnapStoreID)
		if err != nil {
			return []params.SnapStoreResponse{}, errors.Wrap(err, "fetching snap store files")
		}

		var totalAllocated uint64
		for _, file := range files {
			totalAllocated += file.Size
		}
		snapStoreUsage, err := m.snapStoreUsedBytes(val.SnapStoreID)
		if err != nil {
			return []params.SnapStoreResponse{}, errors.Wrap(err, "fetching snap store usage")
		}
		resp[idx] = internalSnapStoreToParamsSnapStore(val)
		resp[idx].StorageUsage = snapStoreUsage
		resp[idx].AllocatedDiskSpace = totalAllocated
	}
	return resp, nil
}

// GetSnapStore returns a snap store identified by ID
func (m *Snapshot) GetSnapStore(storeID string) (params.SnapStoreResponse, error) {
	store, err := m.db.GetSnapStore(storeID)
	if err != nil {
		return params.SnapStoreResponse{}, errors.Wrap(err, "fetcing snap store")
	}
	files, err := m.db.FindSnapStoreFiles(store.SnapStoreID)
	if err != nil {
		return params.SnapStoreResponse{}, errors.Wrap(err, "fetching snap store files")
	}

	var totalAllocated uint64
	for _, file := range files {
		totalAllocated += file.Size
	}
	snapStoreUsage, err := m.snapStoreUsedBytes(store.SnapStoreID)
	if err != nil {
		return params.SnapStoreResponse{}, errors.Wrap(err, "fetching snap store usage")
	}

	resp := internalSnapStoreToParamsSnapStore(store)
	resp.StorageUsage = snapStoreUsage
	resp.AllocatedDiskSpace = totalAllocated
	return resp, nil
}

func (m *Snapshot) RecordSnapStoreFileInDB(snapStoreID string, filePath string, size uint64) error {
	snapStore, err := m.db.GetSnapStore(snapStoreID)
	if err != nil {
		return errors.Wrap(err, "fetching snap store from DB")
	}

	name := path.Base(filePath)

	snapFileParams := db.SnapStoreFile{
		TrackingID:             name,
		SnapStore:              snapStore,
		SnapStoreFilesLocation: snapStore.StorageLocation,
		Path:                   filePath,
		Size:                   size,
	}
	_, err = m.db.CreateSnapStoreFile(snapFileParams)
	if err != nil {
		return errors.Wrap(err, "adding snap store file")
	}

	snapStore.TotalAllocatedSize += size
	if err := m.db.UpdateSnapStore(snapStore); err != nil {
		return errors.Wrap(err, "updating snap store")
	}
	return nil
}

func (m *Snapshot) AddCapacityToSnapStore(snapStoreID string, capacity uint64) error {
	var err error
	snapStore, err := m.db.GetSnapStore(snapStoreID)
	if err != nil {
		return errors.Wrap(err, "fetching snap store from DB")
	}

	locationInfo, err := m.getSnapStoreLoctionInfo(snapStore.StorageLocation)
	if err != nil {
		return errors.Wrap(err, "getting location info")
	}

	if locationInfo.AvailableCapacity < uint64(capacity) {
		return errors.Errorf("Cannot allocate %d bytes for snap store %s. Location only has %d bytes available", capacity, snapStore.StorageLocation.Path, locationInfo.AvailableCapacity)
	}

	newFileName := uuid.New()
	snapStoreFilePath := filepath.Join(snapStore.Path(), newFileName.String())
	if err := util.CreateSnapStoreFile(snapStoreFilePath, capacity); err != nil {
		return errors.Errorf("failed to create %s: %+v", snapStoreFilePath, err)
	}
	defer func() {
		if err != nil {
			os.Remove(snapStoreFilePath)
		}
	}()

	snapFileParams := db.SnapStoreFile{
		TrackingID:             newFileName.String(),
		SnapStore:              snapStore,
		SnapStoreFilesLocation: snapStore.StorageLocation,
		Path:                   snapStoreFilePath,
		Size:                   capacity,
	}

	_, err = m.db.CreateSnapStoreFile(snapFileParams)
	if err != nil {
		return errors.Wrap(err, "adding snap store file")
	}

	defer func() {
		if err != nil {
			m.db.DeleteSnapStoreFile(snapFileParams.TrackingID)
		}
	}()

	snapStoreIDFromString, err := uuid.Parse(snapStore.SnapStoreID)
	if err != nil {
		return errors.Wrap(err, "parsing snap store ID")
	}
	snapStoreParam := types.SnapStore{
		ID: [16]byte(snapStoreIDFromString),
		SnapshotDeviceID: types.DevID{
			Major: snapStore.StorageLocation.Major,
			Minor: snapStore.StorageLocation.Minor,
		},
	}

	if err := ioctl.SnapStoreAddFile(snapStoreParam, snapStoreFilePath); err != nil {
		return errors.Wrap(err, "adding file to snap store")
	}
	snapStore.TotalAllocatedSize += capacity
	if err := m.db.UpdateSnapStore(snapStore); err != nil {
		return errors.Wrap(err, "updating snap store")
	}
	return nil
}
