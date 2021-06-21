package manager

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/timshannon/bolthold"

	"coriolis-veeam-bridge/apiserver/params"
	"coriolis-veeam-bridge/config"
	"coriolis-veeam-bridge/db"
	vErrors "coriolis-veeam-bridge/errors"
	"coriolis-veeam-bridge/internal/ioctl"
	"coriolis-veeam-bridge/internal/storage"
	"coriolis-veeam-bridge/internal/types"
	"coriolis-veeam-bridge/internal/util"
)

type NotificationType string

var (
	SnapStoreCreateEvent NotificationType = "snapStoreCreate"
)

func NewManager(cfg *config.Config) (manager *Snapshot, err error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var dbNeedsInit bool

	if _, err := os.Stat(cfg.DBFile); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, errors.Wrapf(err, "checking db file %s", cfg.DBFile)
		}
		dbNeedsInit = true
	}

	database, err := db.NewDatabase(cfg.DBFile)
	if err != nil {
		return nil, errors.Wrapf(err, "opening database %s", cfg.DBFile)
	}

	snapshotMaganer := &Snapshot{
		cfg:            cfg,
		db:             database,
		notifyChannels: map[NotificationType][]chan interface{}{},
	}
	if dbNeedsInit {
		defer func() {
			// The database requires init, but we failed to initialize
			// state on first run. Delete the newly created DB file, which
			// is not yet properly set up.
			if err != nil && dbNeedsInit {
				os.Remove(cfg.DBFile)
			}
		}()

		err = snapshotMaganer.cleanStorage()
		if err != nil {
			return nil, errors.Wrap(err, "cleaning snap store locations")
		}

	}
	err = snapshotMaganer.addSnapStoreFilesLocations()
	if err != nil {
		return nil, errors.Wrap(err, "adding CoW destinations to db")
	}

	err = snapshotMaganer.initTrackedDisks()
	if err != nil {
		return nil, errors.Wrap(err, "auto adding physical disks to tracking")
	}
	return snapshotMaganer, nil
}

type Snapshot struct {
	cfg            *config.Config
	db             *db.Database
	notifyChannels map[NotificationType][]chan interface{}

	mux sync.Mutex
}

func (m *Snapshot) RegisterNotificationChannel(notifyType NotificationType, ch chan interface{}) {
	m.mux.Lock()
	defer m.mux.Unlock()
	log.Printf("registering new notification channel for %s", notifyType)
	_, ok := m.notifyChannels[notifyType]
	if !ok {
		m.notifyChannels[notifyType] = []chan interface{}{
			ch,
		}
	} else {
		m.notifyChannels[notifyType] = append(m.notifyChannels[notifyType], ch)
	}
}

func (m *Snapshot) SendNotify(notifyType NotificationType, payload interface{}) {
	notify, ok := m.notifyChannels[notifyType]
	if !ok {
		return
	}
	if len(notify) == 0 {
		return
	}

	for _, val := range notify {
		val <- payload
	}
}

// internalBlockVolumeToParamsBlockVolume converts an internal block volume
// to a params block volume. We copy the values because we want to be free to
// change de underlying storage implementation in the future, without changing
// the response the consumers get. I realize this is a premature abstraction,
// but I could not bring myself to add json tags to a structure inside the
// internal package, let alone return it as is to the API consumer.
func internalBlockVolumeToParamsBlockVolume(volume storage.BlockVolume) params.BlockVolume {
	ret := params.BlockVolume{
		Name:               volume.Name,
		Path:               volume.Path,
		PartitionTableType: volume.PartitionTableType,
		PartitionTableUUID: volume.PartitionTableUUID,
		Size:               volume.Size,
		LogicalSectorSize:  volume.LogicalSectorSize,
		PhysicalSectorSize: volume.PhysicalSectorSize,
		FilesystemType:     volume.FilesystemType,
		AlignmentOffset:    volume.AlignmentOffset,
		Major:              volume.Major,
		Minor:              volume.Minor,
		DeviceMapperSlaves: volume.DeviceMapperSlaves,
		IsVirtual:          volume.IsVirtual,
	}
	for _, val := range volume.Partitions {
		ret.Partitions = append(ret.Partitions, internalPartitionToParamsPartition(val))
	}
	return ret
}

func internalPartitionToParamsPartition(partition storage.Partition) params.Partition {
	return params.Partition{
		Name:            partition.Name,
		Path:            partition.Path,
		Sectors:         partition.Sectors,
		FilesystemUUID:  partition.FilesystemUUID,
		PartitionUUID:   partition.PartitionUUID,
		PartitionType:   partition.PartitionType,
		Label:           partition.Label,
		FilesystemType:  partition.FilesystemType,
		StartSector:     partition.StartSector,
		EndSector:       partition.EndSector,
		AlignmentOffset: partition.AlignmentOffset,
		Major:           partition.Major,
		Minor:           partition.Minor,
	}
}

func (m *Snapshot) listDisks(includeVirtual bool) ([]storage.BlockVolume, error) {
	devices, err := storage.BlockDeviceList(false)
	if err != nil {
		return nil, errors.Wrap(err, "listing devices")
	}

	toExclude := m.cfg.CowDestinationDevices()

	var ret []storage.BlockVolume
	for _, val := range devices {
		if !includeVirtual && val.IsVirtual {
			continue
		}

		shouldExclude := false
		for _, cowDevice := range toExclude {
			if cowDevice == val.Path {
				shouldExclude = true
				break
			} else {
				for _, part := range val.Partitions {
					if part.Path == cowDevice {
						shouldExclude = true
						break
					}
				}
			}
		}
		if shouldExclude {
			continue
		}

		ret = append(ret, val)
	}

	return ret, nil
}

func (m *Snapshot) ListDisks(includeVirtual bool) ([]params.BlockVolume, error) {
	devices, err := m.listDisks(includeVirtual)
	if err != nil {
		return nil, errors.Wrap(err, "listing devices")
	}

	ret := make([]params.BlockVolume, len(devices))
	for idx, val := range devices {
		ret[idx] = internalBlockVolumeToParamsBlockVolume(val)
		exists, err := m.db.GetTrackedDisk(val.Major, val.Minor)
		if err != nil {
			if !errors.Is(err, bolthold.ErrNotFound) {
				return nil, errors.Wrap(err, "fetching DB entries")
			}
		} else {
			// We'll never have enough disks to overfllow.
			ret[idx].TrackingID = exists.TrackingID
		}
	}
	return ret, nil
}

///////////////////
// Tracked disks //
///////////////////

func (m *Snapshot) GetTrackedDisk(diskID string) (params.BlockVolume, error) {
	if diskID == "" {
		return params.BlockVolume{}, vErrors.NewBadRequestError("invalid disk id")
	}
	disk, err := m.db.GetTrackedDiskByTrackingID(diskID)
	if err != nil {
		if errors.Is(err, bolthold.ErrNotFound) {
			return params.BlockVolume{}, vErrors.NewNotFoundError("disk with id %s not found", diskID)
		}
		return params.BlockVolume{}, errors.Wrap(err, "fetching from db")
	}

	volume, err := m.findDiskByPath(disk.Path)
	if err != nil {
		return params.BlockVolume{}, errors.Wrap(err, "fetching disk")
	}
	ret := internalBlockVolumeToParamsBlockVolume(volume)
	ret.TrackingID = disk.TrackingID
	return ret, nil
}

func (m *Snapshot) findDiskByPath(path string) (storage.BlockVolume, error) {
	disks, err := m.listDisks(true)
	if err != nil {
		return storage.BlockVolume{}, errors.Wrap(err, "fetching disk list")
	}

	for _, val := range disks {
		if val.Path == path {
			return val, nil
		}
	}

	return storage.BlockVolume{}, vErrors.NewNotFoundError("could not find %s", path)
}

func (m *Snapshot) AddTrackedDisk(disk params.AddTrackedDiskRequest) (params.BlockVolume, error) {
	m.mux.Lock()
	defer m.mux.Unlock()

	volume, err := m.findDiskByPath(disk.DevicePath)
	if err != nil {
		return params.BlockVolume{}, errors.Wrap(err, "fetching disk")
	}

	if volume.Path == "" {
		return params.BlockVolume{}, vErrors.NewNotFoundError("device %s not found", disk.DevicePath)
	}

	exists, err := m.db.GetTrackedDisk(volume.Major, volume.Minor)
	if err != nil {
		if !errors.Is(err, bolthold.ErrNotFound) {
			return params.BlockVolume{}, errors.Wrap(err, "fetching DB entries")
		}
	}

	cbtInfo, err := ioctl.GetCBTInfo()
	if err != nil {
		return params.BlockVolume{}, errors.Wrap(err, "fetching CBT info")
	}

	devID := types.DevID{
		Major: volume.Major,
		Minor: volume.Minor,
	}

	if !deviceIsTracked(volume.Major, volume.Minor, cbtInfo) {
		log.Printf("Adding %s to tracking", volume.Path)
		if err := ioctl.AddDeviceToTracking(devID); err != nil {
			log.Printf("error adding %s to tracking: %s", volume.Path, err)
			return params.BlockVolume{}, errors.Wrapf(err, "adding %s to tracking", volume.Path)
		}
	}

	var dbObject db.TrackedDisk
	if exists == (db.TrackedDisk{}) {
		addDevParams := db.TrackedDisk{
			TrackingID: filepath.Base(volume.Path),
			Path:       volume.Path,
			Major:      volume.Major,
			Minor:      volume.Minor,
		}

		dbObject, err = m.db.CreateTrackedDisk(addDevParams)
		if err != nil {
			return params.BlockVolume{}, errors.Wrapf(err, "adding db entry for %s", volume.Path)
		}
	} else {
		dbObject = exists
	}

	ret := internalBlockVolumeToParamsBlockVolume(volume)
	ret.TrackingID = dbObject.TrackingID
	return ret, nil
}

//////////////////////////////
// Snap store file location //
//////////////////////////////

// AddSnapStoreFilesLocation creates a new snap store location. Locations hosted on a device
// that is currently tracked, will err out.
func (m *Snapshot) AddSnapStoreFilesLocation(path string) (params.SnapStoreLocation, error) {
	fsInfo, err := util.GetFileSystemInfoFromPath(path)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching filesystem info")
	}

	deviceInfo, err := util.GetBlockDeviceInfoFromFile(path)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching device info")
	}

	devID := types.DevID{
		Major: deviceInfo.Major,
		Minor: deviceInfo.Minor,
	}
	allInvolvedDevices, err := util.FindAllInvolvedDevices([]types.DevID{devID})
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "finding all devices")
	}

	allTrackedDisks, err := m.db.GetAllTrackedDisks()
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching tracked disks")
	}

	for _, tracked := range allTrackedDisks {
		for _, involved := range allInvolvedDevices {
			if involved == tracked.Path {
				return params.SnapStoreLocation{}, vErrors.NewConflictError("location %s is on tracked disk %s", path, involved)
			}
		}
	}

	_, err = m.db.GetSnapStoreFilesLocation(path)
	if err != nil {
		if !errors.Is(err, vErrors.ErrNotFound) {
			return params.SnapStoreLocation{}, errors.Wrap(err, "fetching snap store location info")
		}
	} else {
		return params.SnapStoreLocation{}, vErrors.NewConflictError("location already exists")
	}

	newLocParams := db.SnapStoreFilesLocation{
		Path:          path,
		TotalCapacity: fsInfo.Blocks * uint64(fsInfo.BlockSize),
		DevicePath:    deviceInfo.DevicePath,
		Major:         deviceInfo.Major,
		Minor:         deviceInfo.Minor,
		Enabled:       true,
	}

	createdStore, err := m.db.CreateSnapStoreFileLocation(newLocParams)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "creating db entry")
	}

	files, err := m.db.FindSnapStoreLocationFiles(createdStore.TrackingID)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching snap store files")
	}
	var totalAllocated uint64
	for _, file := range files {
		totalAllocated += file.Size
	}

	return params.SnapStoreLocation{
		ID:                createdStore.TrackingID,
		AllocatedCapacity: totalAllocated,
		AvailableCapacity: fsInfo.BlocksAvailable * uint64(fsInfo.BlockSize),
		TotalCapacity:     createdStore.TotalCapacity,
		Path:              createdStore.Path,
		DevicePath:        createdStore.DevicePath,
		Major:             createdStore.Major,
		Minor:             createdStore.Minor,
	}, nil
}

func (m *Snapshot) getSnapStoreLoctionInfo(location db.SnapStoreFilesLocation) (params.SnapStoreLocation, error) {
	fsInfo, err := util.GetFileSystemInfoFromPath(location.Path)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching filesystem info")
	}

	files, err := m.db.FindSnapStoreLocationFiles(location.TrackingID)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching snap store files")
	}
	var totalAllocated uint64
	for _, file := range files {
		totalAllocated += file.Size
	}

	return params.SnapStoreLocation{
		ID:                location.TrackingID,
		AllocatedCapacity: totalAllocated,
		AvailableCapacity: fsInfo.BlocksAvailable * uint64(fsInfo.BlockSize),
		TotalCapacity:     location.TotalCapacity,
		Path:              location.Path,
		DevicePath:        location.DevicePath,
		Major:             location.Major,
		Minor:             location.Minor,
	}, nil
}

func (m *Snapshot) GetSnapStoreFilesLocation(path string) (params.SnapStoreLocation, error) {
	dbSnapFileDestination, err := m.db.GetSnapStoreFilesLocation(path)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching snap store location info")
	}

	return m.getSnapStoreLoctionInfo(dbSnapFileDestination)
}

func (m *Snapshot) GetSnapStoreFilesLocationByID(locationID string) (params.SnapStoreLocation, error) {
	dbSnapFileDestination, err := m.db.GetSnapStoreFilesLocationByID(locationID)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching snap store location info")
	}

	return m.getSnapStoreLoctionInfo(dbSnapFileDestination)
}

func (m *Snapshot) ListAvailableSnapStoreLocations() ([]params.SnapStoreLocation, error) {
	ret := make([]params.SnapStoreLocation, len(m.cfg.CoWDestination))

	snapStoreFilesLocations, err := m.db.ListSnapStoreFilesLocations()
	if err != nil {
		return nil, errors.Wrap(err, "listing snap store files locations")
	}

	for idx, val := range snapStoreFilesLocations {
		fsInfo, err := util.GetFileSystemInfoFromPath(val.Path)
		if err != nil {
			return nil, errors.Wrap(err, "fetching filesystem info")
		}

		files, err := m.db.FindSnapStoreLocationFiles(val.TrackingID)
		if err != nil {
			return []params.SnapStoreLocation{}, errors.Wrap(err, "fetching snap store files")
		}
		var totalAllocated uint64
		for _, file := range files {
			totalAllocated += file.Size
		}

		ret[idx] = params.SnapStoreLocation{
			ID:                val.TrackingID,
			AllocatedCapacity: totalAllocated,
			AvailableCapacity: fsInfo.BlocksAvailable * uint64(fsInfo.BlockSize),
			TotalCapacity:     val.TotalCapacity,
			Path:              val.Path,
			DevicePath:        val.DevicePath,
			Major:             val.Major,
			Minor:             val.Minor,
		}
	}
	return ret, nil
}

/////////////////
// Snap stores //
/////////////////

func (m *Snapshot) snapStoreUsedBytes(snapStoreID string) (uint64, error) {
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

	log.Printf("fetching info about %s", snapStoreID)
	snapStoreRet, err := ioctl.SnapStoreCleanup(snapStoreParams)
	if err != nil {
		return 0, errors.Wrap(err, "fetching snap store usage")
	}
	if snapStoreRet.FilledBytes == ioctl.SNAP_STORE_NOT_FOUND {
		return 0, vErrors.NewNotFoundError("snap store %s does not exist", snapStoreID)
	}
	log.Printf("Filled bytes for %s is %d", snapStoreID, snapStoreRet.FilledBytes)
	return snapStoreRet.FilledBytes, nil
}

// CreateSnapStore creates a new snap store
func (m *Snapshot) CreateSnapStore(trackedDisk, storeLocation string) (params.SnapStoreResponse, error) {
	var err error

	_, err = m.db.FindSnapStoresForDevice(trackedDisk)
	if err != nil {
		if !errors.Is(err, bolthold.ErrNotFound) {
			return params.SnapStoreResponse{}, errors.Wrap(err, "fetching snap store")
		}
	} else {
		return params.SnapStoreResponse{}, vErrors.NewConflictError("device %s already has a snap store", trackedDisk)
	}

	snapStoreLocation, err := m.db.GetSnapStoreFilesLocationByID(storeLocation)
	if err != nil {
		return params.SnapStoreResponse{}, errors.Wrap(err, "fetching snap store location")
	}

	disk, err := m.db.GetTrackedDiskByTrackingID(trackedDisk)
	if err != nil {
		if errors.Is(err, bolthold.ErrNotFound) {
			return params.SnapStoreResponse{}, vErrors.NewNotFoundError("no such tracked disk: %s", trackedDisk)
		}
		return params.SnapStoreResponse{}, errors.Wrap(err, "fetching tracked disk")
	}

	deviceIDs := []types.DevID{
		{
			Major: disk.Major,
			Minor: disk.Minor,
		},
	}

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
		return params.SnapStoreResponse{}, errors.Wrap(err, "adding store to db")
	}

	defer func() {
		if err != nil {
			m.db.DeleteSnapStore(store.SnapStoreID)
		}
	}()

	if store.Path() == "" {
		return params.SnapStoreResponse{}, errors.Errorf("snap store is invalid")
	}

	log.Printf("Checking for snap store location folder: %s", store.Path())
	if _, statErr := os.Stat(store.Path()); statErr != nil {
		if !errors.Is(statErr, fs.ErrNotExist) {
			return params.SnapStoreResponse{}, errors.Wrap(statErr, "checking storage location")
		}
		log.Printf("Creating snap store location folder: %s", store.Path())

		if mkdirErr := os.MkdirAll(store.Path(), 00770); mkdirErr != nil {
			log.Printf("Error creating snap store location folder %s: %q", store.Path(), mkdirErr)
			return params.SnapStoreResponse{}, errors.Wrap(mkdirErr, "creating storage location")
		}

		defer func(storageNeedsInit bool) {
			if err != nil && storageNeedsInit {
				log.Printf("Cleaning snap store location folder %s due to error: %q", store.Path(), err)
				os.RemoveAll(store.Path())
			}
		}(true)
	}

	snapStore, err := ioctl.CreateSnapStore(uuidAsBytes, deviceIDs, snapDisk)
	if err != nil {
		return params.SnapStoreResponse{}, errors.Wrap(err, "creating snap store")
	}

	snapStoreID := uuid.UUID(snapStore.ID)

	if snapStoreID != newUUID {
		panic("returned snap store ID missmatch")
	}

	snapStoreRet := params.SnapStoreResponse{
		ID:                 store.SnapStoreID,
		TrackedDiskID:      store.TrackedDisk.TrackingID,
		StorageLocationID:  store.StorageLocation.TrackingID,
		AllocatedDiskSpace: store.TotalAllocatedSize,
	}

	m.SendNotify(SnapStoreCreateEvent, snapStoreRet)
	return snapStoreRet, nil
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
		resp[idx] = params.SnapStoreResponse{
			ID:                 val.SnapStoreID,
			TrackedDiskID:      val.TrackedDisk.TrackingID,
			StorageLocationID:  val.StorageLocation.TrackingID,
			AllocatedDiskSpace: totalAllocated,
			StorageUsage:       snapStoreUsage,
		}
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

	resp := params.SnapStoreResponse{
		ID:                 store.SnapStoreID,
		TrackedDiskID:      store.TrackedDisk.TrackingID,
		StorageLocationID:  store.StorageLocation.TrackingID,
		AllocatedDiskSpace: totalAllocated,
		StorageUsage:       snapStoreUsage,
	}
	return resp, nil
}

func (m *Snapshot) AddCapacityToSnapStore(snapStoreID string, capacity uint64) error {
	var err error
	snapStore, err := m.db.GetSnapStore(snapStoreID)
	if err != nil {
		return errors.Wrap(err, "fetching snap store from DB")
	}

	location, err := m.db.GetSnapStoreFilesLocationByID(snapStore.StorageLocation.TrackingID)
	if err != nil {
		return errors.Wrap(err, "fetching storage location")
	}

	locationInfo, err := m.getSnapStoreLoctionInfo(location)
	if err != nil {
		return errors.Wrap(err, "getting location info")
	}

	if locationInfo.AvailableCapacity < uint64(capacity) {
		return errors.Errorf("Cannot allocate %d bytes for snap store %s. Location only has %d bytes available", capacity, location.TrackingID, locationInfo.AvailableCapacity)
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
		SnapStoreFilesLocation: location,
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

	return nil
}

////////////////////////
// Snap store mapping //
////////////////////////

func (m *Snapshot) CreateSnapStoreMapping(param params.CreateSnapStoreMappingRequest) (params.SnapStoreMappingResponse, error) {
	trackedDisk, err := m.db.GetTrackedDiskByTrackingID(param.TrackedDisk)
	if err != nil {
		return params.SnapStoreMappingResponse{}, errors.Wrap(err, "fetching tracked disk")
	}

	existingStore, err := m.db.GetSnapStoreMappingByDeviceID(param.TrackedDisk)
	if err != nil {
		if !errors.Is(err, vErrors.ErrNotFound) {
			return params.SnapStoreMappingResponse{}, errors.Wrap(err, "checking store mapping")
		}
	} else {
		return params.SnapStoreMappingResponse{}, vErrors.NewConflictError("disk %s already has a snap store mapping: %s", trackedDisk.TrackingID, existingStore.TrackingID)
	}

	storeLocation, err := m.db.GetSnapStoreFilesLocationByID(param.SnapStoreLocation)
	if err != nil {
		return params.SnapStoreMappingResponse{}, errors.Wrap(err, "fetching store location")
	}

	newID := uuid.New()
	createParams := db.SnapStoreMapping{
		TrackingID:             newID.String(),
		TrackedDisk:            trackedDisk,
		SnapStoreFilesLocation: storeLocation,
	}

	newMappingRet, err := m.db.CreateSnapStoreMapping(createParams)
	if err != nil {
		return params.SnapStoreMappingResponse{}, errors.Wrap(err, "creating mapping")
	}
	return params.SnapStoreMappingResponse{
		ID:                newMappingRet.TrackingID,
		TrackedDiskID:     trackedDisk.TrackingID,
		StorageLocationID: storeLocation.TrackingID,
	}, nil
}

func (m *Snapshot) ListSnapStoreMappings() ([]params.SnapStoreMappingResponse, error) {
	storeMappings, err := m.db.ListSnapStoreMappings()
	if err != nil {
		return []params.SnapStoreMappingResponse{}, errors.Wrap(err, "fetching snap store mappings")
	}
	ret := make([]params.SnapStoreMappingResponse, len(storeMappings))
	for idx, val := range storeMappings {
		ret[idx] = params.SnapStoreMappingResponse{
			ID:                val.TrackingID,
			TrackedDiskID:     val.TrackedDisk.TrackingID,
			StorageLocationID: val.SnapStoreFilesLocation.TrackingID,
		}
	}
	return ret, nil
}

///////////////
// Snapshots //
///////////////

// CreateSnapshot creates a new snapshot of one or more disks.
func (m *Snapshot) CreateSnapshot(param params.CreateSnapshotRequest) error {
	m.mux.Lock()
	defer m.mux.Unlock()

	info, err := ioctl.GetCBTInfo()
	if err != nil {
		fmt.Printf("%+v\n", err)
		return errors.Wrap(err, "fetching CBT info")
	}
	js, _ := json.MarshalIndent(info, "", "  ")
	fmt.Println(string(js))

	imgs, err := ioctl.CollectSnapshotImages()
	if err != nil {
		return errors.Wrap(err, "collecting images")
	}

	jsImg, _ := json.MarshalIndent(imgs, "", "  ")
	fmt.Println(string(jsImg))

	// js, _ := json.MarshalIndent(cbtInfo, "", "  ")
	// fmt.Println(string(js))

	var devices []types.DevID
	for _, val := range param.TrackedDiskIDs {
		dbDev, err := m.db.GetTrackedDiskByTrackingID(val)
		if err != nil {
			return errors.Wrap(err, "getting device")
		}
		devices = append(devices, types.DevID{
			Major: dbDev.Major,
			Minor: dbDev.Minor,
		})
	}

	snapshot, err := ioctl.CreateSnapshot(devices)
	if err != nil {
		fmt.Printf(">>> %+v\n", err)
		return errors.Wrap(err, "creating snapshot")
	}

	js2, _ := json.MarshalIndent(snapshot, "", "  ")
	fmt.Println(string(js2))

	info, err = ioctl.GetCBTInfo()
	if err != nil {
		fmt.Printf("%+v\n", err)
		return errors.Wrap(err, "fetching CBT info")
	}
	js, _ = json.MarshalIndent(info, "", "  ")
	fmt.Println(string(js))

	imgs, err = ioctl.CollectSnapshotImages()
	if err != nil {
		return errors.Wrap(err, "collecting images")
	}

	jsImg, _ = json.MarshalIndent(imgs, "", "  ")
	fmt.Println(string(jsImg))

	// if err := ioctl.DeleteSnapshot(snapshot.SnapshotID); err != nil {
	// 	return errors.Wrap(err, "deleting snapshots")
	// }

	// bitmap, err := ioctl.GetCBTBitmap(params)

	// if err != nil {
	// 	fmt.Printf("%+v\n", err)
	// 	return
	// }
	return nil
}

func (m *Snapshot) DeleteSnaphot(snapshotID uint64) error {
	return nil
}

// Init functions.
// These functions should only be run *once* when the
// service is first started after a reboot.

// cleanStorage will remove all files and folders inside the configured
// CoWDestination array specified in the config.
func (m *Snapshot) cleanStorage() error {
	for _, val := range m.cfg.CoWDestination {
		if _, err := os.Stat(val); err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return errors.Wrapf(err, "checking CoWDestination %s", val)
			}
			if err := os.MkdirAll(val, 00770); err != nil {
				return errors.Wrapf(err, "creating %s", val)
			}
			// We created the folder, there is nothing to clean. Continue.
			continue
		}
		files, err := ioutil.ReadDir(val)
		if err != nil {
			return errors.Wrapf(err, "reading %s", val)
		}

		for _, item := range files {
			fullPath := filepath.Join(val, item.Name())
			if err := os.RemoveAll(fullPath); err != nil {
				return errors.Wrapf(err, "removing %s", fullPath)
			}
		}
	}
	return nil
}

func deviceIsTracked(major, minor uint32, cbtInfo []types.CBTInfo) bool {
	for _, cbt := range cbtInfo {
		if cbt.DevID.Major == major && cbt.DevID.Minor == minor {
			if cbt.CBTMapSize > 0 {
				info, _ := json.MarshalIndent(cbt, "", "  ")
				log.Print(fmt.Sprintf("device is already tracked: %s", string(info)))
				return true
			}
		}
	}
	return false
}

// initTrackedDisks will add all physical disks that do not take part in
// hosting snap store files, to tracking.
func (m *Snapshot) initTrackedDisks() (err error) {
	if !m.cfg.AutoInitPhysicalDisks {
		return nil
	}
	// listDisks excludes disks configured as snap store destinations.
	disks, err := m.listDisks(false)
	if err != nil {
		return errors.Wrap(err, "fetching disks list")
	}

	for _, val := range disks {
		log.Printf("checking disk %s\n", val.Path)
		newDevParams := params.AddTrackedDiskRequest{
			DevicePath: val.Path,
		}

		_, err = m.AddTrackedDisk(newDevParams)
		if err != nil {
			return errors.Wrapf(err, "adding disk %s to tracking", val.Path)
		}
	}
	return nil
}

// addSnapStoreFilesLocations adds all configured CoWDestination members
// to the database.
func (m *Snapshot) addSnapStoreFilesLocations() error {
	for _, val := range m.cfg.CoWDestination {
		if _, err := m.AddSnapStoreFilesLocation(val); err != nil {
			if !errors.Is(err, &vErrors.ConflictError{}) {
				return errors.Wrap(err, "creating snap store location")
			}
		}
	}
	return nil
}

func (m *Snapshot) PopulateSnapStoreWatcher() error {
	stores, err := m.ListSnapStores()
	if err != nil {
		return errors.Wrap(err, "initializing snap storage worker")
	}

	for _, store := range stores {
		m.SendNotify(SnapStoreCreateEvent, store)
	}
	return nil
}

// End init functions.
