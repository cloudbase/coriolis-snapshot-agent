package db

import (
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/timshannon/bolthold"
	"go.etcd.io/bbolt"

	vErrors "coriolis-veeam-bridge/errors"
	"coriolis-veeam-bridge/internal/types"
)

// Open opens the database at path and returns a *bolt.DB object
func Open(path string) (*bolthold.Store, error) {
	bboltOptions := bbolt.Options{
		Timeout: 1 * time.Second,
	}
	db, err := bolthold.Open(path, 0600, &bolthold.Options{Options: &bboltOptions})
	if err != nil {
		return nil, errors.Wrap(err, "opening database")
	}
	return db, nil
}

// NewDatabase returns a new *Database object
func NewDatabase(dbFile string) (*Database, error) {
	con, err := Open(dbFile)
	if err != nil {
		return nil, errors.Wrap(err, "opening databse file")
	}
	cfg := &Database{
		location: dbFile,
		con:      con,
	}

	return cfg, nil
}

// Database is the database interface to the bold db
type Database struct {
	location string
	con      *bolthold.Store
}

/////////////////
// TrackedDisk //
/////////////////
// GetTrackedDisk gets one tracked disk entity from the database
func (d *Database) GetTrackedDisk(major, minor uint32) (TrackedDisk, error) {
	var trackedDisk TrackedDisk

	if err := d.con.FindOne(&trackedDisk, bolthold.Where("Major").Eq(major).And("Minor").Eq(minor)); err != nil {
		return TrackedDisk{}, errors.Wrap(err, "fetching db entries")
	}
	return trackedDisk, nil
}

// GetTrackedDisk gets one tracked disk entity from the database
func (d *Database) GetTrackedDiskByTrackingID(trackingID string) (TrackedDisk, error) {
	var trackedDisk TrackedDisk

	if err := d.con.FindOne(&trackedDisk, bolthold.Where("TrackingID").Eq(trackingID)); err != nil {
		return TrackedDisk{}, errors.Wrap(err, "fetching tracked disk by id")
	}
	return trackedDisk, nil
}

// GetAllTrackedDisks fetches all tracked disk entities from the database.
func (d *Database) GetAllTrackedDisks() ([]TrackedDisk, error) {
	var allTracked []TrackedDisk
	re := regexp.MustCompile(".*")
	if err := d.con.Find(&allTracked, bolthold.Where("TrackingID").RegExp(re)); err != nil {
		return nil, errors.Wrap(err, "fetching all tracked disks")
	}
	return allTracked, nil
}

// CreateTrackedDisk adds a new tracked disk entity to the database.
func (d *Database) CreateTrackedDisk(device TrackedDisk) (TrackedDisk, error) {
	if err := d.con.Insert(device.TrackingID, &device); err != nil {
		return TrackedDisk{}, errors.Wrap(err, "inserting new snap store location into db")
	}
	return device, nil
}

// RemoveTrackedDisk removes a tracked disk entity from the database.
func (d *Database) RemoveTrackedDisk(device types.DevID) error {
	return nil
}

///////////////
// Snapshots //
///////////////
// GetSnapshot gets one snapshot entity, identified by snapID, from the database.
func (d *Database) GetSnapshot(snapID uint64) (Snapshot, error) {
	var snapshot Snapshot
	if err := d.con.FindOne(&snapshot, bolthold.Where("SnapshotID").Eq(snapID)); err != nil {
		if errors.Is(err, bolthold.ErrNotFound) {
			return snapshot, vErrors.NewNotFoundError("snapshot ID %s not found in db", snapID)
		}
		return snapshot, errors.Wrap(err, "finding location in db")
	}
	return snapshot, nil
}

func (d *Database) ListSnapshotsForDisk(diskID string) ([]Snapshot, error) {
	var volumeSnaps []VolumeSnapshot
	if err := d.con.Find(&volumeSnaps, bolthold.Where("OriginalDevice.TrackingID").Eq(diskID)); err != nil {
		return []Snapshot{}, errors.Wrap(err, "fetching records")
	}

	var snapshots []Snapshot
	snapshotIDs := make([]interface{}, len(volumeSnaps))
	for idx, val := range volumeSnaps {
		snapshotIDs[idx] = val.SnapshotID
	}

	if err := d.con.Find(&snapshots, bolthold.Where("SnapshotID").In(snapshotIDs)); err != nil {
		return []Snapshot{}, errors.Wrap(err, "fetching records")
	}

	return snapshots, nil
}

// ListAllSnapshots lists all snapshots from the database.
func (d *Database) ListAllSnapshots() ([]Snapshot, error) {
	var snapshots []Snapshot

	re := regexp.MustCompile(".*")
	if err := d.con.Find(&snapshots, bolthold.Where("SnapshotID").RegExp(re)); err != nil {
		return []Snapshot{}, errors.Wrap(err, "fetching records")
	}

	return snapshots, nil
}

// CreateSnapshot creates a new snapshot entity inside the database.
func (d *Database) CreateSnapshot(param Snapshot) (Snapshot, error) {
	if err := d.con.Insert(param.SnapshotID, &param); err != nil {
		return Snapshot{}, errors.Wrap(err, "inserting new snap store location into db")
	}
	return param, nil
}

// DeleteSnapshot deletes a snapshot entity from the databse.
func (d *Database) DeleteSnapshot(snapshotID uint64) error {
	return nil
}

/////////////////
// Snap stores //
/////////////////
// GetSnapStore fetches one snap store entity from the database.
func (d *Database) GetSnapStore(storeID string) (SnapStore, error) {
	var store SnapStore
	if err := d.con.FindOne(&store, bolthold.Where("SnapStoreID").Eq(storeID)); err != nil {
		if errors.Is(err, bolthold.ErrNotFound) {
			return store, vErrors.NewNotFoundError("store ID %s not found in db", storeID)
		}
		return store, errors.Wrap(err, "finding location in db")
	}
	return store, nil
}

// GetSnapStore fetches one snap store entity from the database.
func (d *Database) GetSnapStoreByDiskID(diskID string) (SnapStore, error) {
	var store SnapStore
	if err := d.con.FindOne(&store, bolthold.Where("TrackedDisk.TrackingID").Eq(diskID)); err != nil {
		if errors.Is(err, bolthold.ErrNotFound) {
			return store, vErrors.NewNotFoundError("store not found in disk ID %s", diskID)
		}
		return store, errors.Wrap(err, "finding location in db")
	}
	return store, nil
}

// ListSnapStores fetches all snap store entities from the database.
func (d *Database) ListSnapStores() ([]SnapStore, error) {
	var stores []SnapStore
	re := regexp.MustCompile(".*")
	if err := d.con.Find(&stores, bolthold.Where("SnapStoreID").RegExp(re)); err != nil {
		return nil, errors.Wrap(err, "fetching records")
	}
	return stores, nil
}

func (d *Database) FindSnapStoreFiles(storeID string) ([]SnapStoreFile, error) {
	var files []SnapStoreFile
	if err := d.con.Find(&files, bolthold.Where("SnapStore.SnapStoreID").Eq(storeID)); err != nil {
		return nil, errors.Wrap(err, "fetching files")
	}
	return files, nil
}

// CreateSnapStore creates a new snap store entity inside the database.
func (d *Database) CreateSnapStore(param SnapStore) (SnapStore, error) {
	if err := d.con.Insert(param.SnapStoreID, &param); err != nil {
		return SnapStore{}, errors.Wrap(err, "inserting new snap store into db")
	}
	return param, nil
}

func (d *Database) FindSnapStoresForDevice(trackedDiskID string) (SnapStore, error) {
	var snapStore SnapStore
	if err := d.con.FindOne(&snapStore, bolthold.Where("TrackedDisk.TrackingID").Eq(trackedDiskID)); err != nil {
		return snapStore, errors.Wrap(err, "fetching snap store for device")
	}
	return snapStore, nil
}

// DeleteSnapStore deletes a snap store from the database. This should only
// be used as a cleanup step in case the snap store fails to get created in the veeam module.
func (d *Database) DeleteSnapStore(snapStoreID string) error {
	param := SnapStore{}
	if err := d.con.Delete(snapStoreID, &param); err != nil {
		return errors.Wrap(err, "deleting snap store from db")
	}
	return nil
}

/////////////////////
// Snap store file //
/////////////////////
// CreateSnapStoreFile creates a new snap store file in the db.
func (d *Database) CreateSnapStoreFile(param SnapStoreFile) (SnapStoreFile, error) {
	if err := d.con.Insert(param.TrackingID, &param); err != nil {
		return SnapStoreFile{}, errors.Wrap(err, "inserting new snap store into db")
	}
	return param, nil
}

// DeleteSnapStoreFile deletes a snap store file from the database. Do not expose deleting snap store
// files to API consumers. This operation is only meant as a cleanup step in case of a failure during ioctl.
func (d *Database) DeleteSnapStoreFile(fileID string) error {
	param := SnapStoreFile{}
	if err := d.con.Delete(fileID, &param); err != nil {
		return errors.Wrap(err, "deleting snap store file from db")
	}
	return nil
}

// GetSnapStoreFile gets one snap store file from the database.
func (d *Database) GetSnapStoreFile(path string) (SnapStoreFile, error) {
	return SnapStoreFile{}, nil
}

// ListSnapStoreFiles lists all snap store files in a particular snap store files location
func (d *Database) ListSnapStoreFiles(location SnapStoreFilesLocation) ([]SnapStoreFile, error) {
	return nil, nil
}

// ListAllSnapStoreFiles lists all snap store files we keep track off, regardless of location.
func (d *Database) ListAllSnapStoreFiles() ([]SnapStoreFile, error) {
	return nil, nil
}

func (d *Database) FindSnapStoreLocationFiles(storeLocationID string) ([]SnapStoreFile, error) {
	var files []SnapStoreFile
	if err := d.con.Find(&files, bolthold.Where("SnapStoreFilesLocation.TrackingID").Eq(storeLocationID)); err != nil {
		return nil, errors.Wrap(err, "fetching location files")
	}
	return files, nil
}

/////////////////////////
// Snap store location //
/////////////////////////
// GetSnapStoreFilesLocation gets one snap store file location entity, identified by path, from the database.
func (d *Database) GetSnapStoreFilesLocation(path string) (SnapStoreFilesLocation, error) {
	var location SnapStoreFilesLocation

	if err := d.con.FindOne(&location, bolthold.Where("Path").Eq(path)); err != nil {
		if errors.Is(err, bolthold.ErrNotFound) {
			return SnapStoreFilesLocation{}, vErrors.NewNotFoundError("path %s not found in db", path)
		}
		return SnapStoreFilesLocation{}, errors.Wrap(err, "finding location in db")
	}
	return location, nil
}

// GetSnapStoreFilesLocation gets one snap store file location entity, identified by path, from the database.
func (d *Database) GetSnapStoreFilesLocationByID(trackingID string) (SnapStoreFilesLocation, error) {
	var location SnapStoreFilesLocation

	if err := d.con.FindOne(&location, bolthold.Where("TrackingID").Eq(trackingID)); err != nil {
		if errors.Is(err, bolthold.ErrNotFound) {
			return SnapStoreFilesLocation{}, vErrors.NewNotFoundError("path %s not found in db", trackingID)
		}
		return SnapStoreFilesLocation{}, errors.Wrap(err, "finding location in db")
	}
	return location, nil
}

// CreateSnapStoreFileLocation creates a new snap store file location
func (d *Database) CreateSnapStoreFileLocation(snapStore SnapStoreFilesLocation) (SnapStoreFilesLocation, error) {
	newUUID := uuid.New()
	snapStore.TrackingID = newUUID.String()
	if err := d.con.Insert(newUUID.String(), &snapStore); err != nil {
		return SnapStoreFilesLocation{}, errors.Wrap(err, "inserting new snap store location into db")
	}
	return snapStore, nil
}

// ListSnapStoreFilesLocations lists all known snap store files locations.
func (d *Database) ListSnapStoreFilesLocations() ([]SnapStoreFilesLocation, error) {
	var allLocations []SnapStoreFilesLocation

	re := regexp.MustCompile(".*")
	if err := d.con.Find(&allLocations, bolthold.Where("TrackingID").RegExp(re)); err != nil {
		return nil, errors.Wrap(err, "fetching db entries")
	}
	return allLocations, nil
}

////////////////////////
// Snap store mapping //
////////////////////////

func (d *Database) GetSnapStoreMappingByDeviceID(deviceID string) (SnapStoreMapping, error) {
	var mapping SnapStoreMapping

	if err := d.con.FindOne(&mapping, bolthold.Where("TrackedDisk.TrackingID").Eq(deviceID)); err != nil {
		if errors.Is(err, bolthold.ErrNotFound) {
			return SnapStoreMapping{}, vErrors.NewNotFoundError("mapping for device ID %s not found in db", deviceID)
		}
		return SnapStoreMapping{}, errors.Wrap(err, "finding mapping in db")
	}
	return mapping, nil
}

func (d *Database) GetSnapStoreMappingByLocationID(locationID string) (SnapStoreMapping, error) {
	return SnapStoreMapping{}, nil
}

func (d *Database) GetSnapStoreMappingByID(trackingID string) (SnapStoreMapping, error) {
	return SnapStoreMapping{}, nil
}

func (d *Database) CreateSnapStoreMapping(param SnapStoreMapping) (SnapStoreMapping, error) {
	if err := d.con.Insert(param.TrackingID, &param); err != nil {
		return SnapStoreMapping{}, errors.Wrap(err, "inserting new snap store into db")
	}
	return param, nil
}

func (d *Database) ListSnapStoreMappings() ([]SnapStoreMapping, error) {
	var storeMappings []SnapStoreMapping
	re := regexp.MustCompile(".*")
	if err := d.con.Find(&storeMappings, bolthold.Where("TrackingID").RegExp(re)); err != nil {
		return nil, errors.Wrap(err, "fetching snap store mappings")
	}
	return storeMappings, nil
}

func (d *Database) DeleteSnapStoreMapping(trackingID string) error {
	var storeMapping SnapStoreMapping
	if err := d.con.Delete(trackingID, &storeMapping); err != nil {
		return errors.Wrap(err, "deleting snap store mapping from db")
	}
	return nil
}
