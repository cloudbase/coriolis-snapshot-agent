package db

import (
	"coriolis-veeam-bridge/internal/types"
	"time"

	"github.com/pkg/errors"
	"github.com/timshannon/bolthold"
	"go.etcd.io/bbolt"
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

// GetTrackedDisk gets one tracked disk entity from the database
func (d *Database) GetTrackedDisk(device types.DevID) (TrackedDisk, error) {
	return TrackedDisk{}, nil
}

// GetAllTrackedDisks fetches all tracked disk entities from the database.
func (d *Database) GetAllTrackedDisks() ([]TrackedDisk, error) {
	return nil, nil
}

// CreateTrackedDisk adds a new tracked disk entity to the database.
func (d *Database) CreateTrackedDisk(device types.DevID) (TrackedDisk, error) {
	return TrackedDisk{}, nil
}

// RemoveTrackedDisk removes a tracked disk entity from the database.
func (d *Database) RemoveTrackedDisk(device types.DevID) error {
	return nil
}

// GetSnapshot gets one snapshot entity, identified by snapID, from the database.
func (d *Database) GetSnapshot(snapID uint64) (Snapshot, error) {
	return Snapshot{}, nil
}

// ListSnapshotsForDevice lists all snapshots associated with a device.
func (d *Database) ListSnapshotsForDevice(device types.DevID) ([]Snapshot, error) {
	return nil, nil
}

// ListAllSnapshots lists all snapshots from the database.
func (d *Database) ListAllSnapshots() ([]Snapshot, error) {
	return nil, nil
}

// CreateSnapshot creates a new snapshot entity inside the database.
func (d *Database) CreateSnapshot(devices []types.DevID) (Snapshot, error) {
	return Snapshot{}, nil
}

// DeleteSnapshot deletes a snapshot entity from the databse.
func (d *Database) DeleteSnapshot(snapshotID uint64) error {
	return nil
}

// GetSnapStore fetches one snap store entity from the database.
func (d *Database) GetSnapStore(storeID string) (SnapStore, error) {
	return SnapStore{}, nil
}

// CreateSnapStore creates a new snap store entity inside the database.
func (d *Database) CreateSnapStore(trackedDisk types.DevID, snapDevice string) (SnapStore, error) {
	return SnapStore{}, nil
}

// AddFileToSnapStore associates one snap store file with a snap store, inside the database.
func (d *Database) AddFileToSnapStore(snapStore SnapStore, file SnapStoreFile) error {
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

// GetSnapStoreFilesLocation gets one snap store file location entity, identified by path, from the database.
func (d *Database) GetSnapStoreFilesLocation(path string) (SnapStoreFilesLocation, error) {
	return SnapStoreFilesLocation{}, nil
}

// CreateSnapStoreFileLocation creates a new snap store file location
func (d *Database) CreateSnapStoreFileLocation(snapStore SnapStoreFilesLocation) (SnapStoreFilesLocation, error) {
	return SnapStoreFilesLocation{}, nil
}

// ListSnapStoreFilesLocations lists all known snap store files locations.
func (d *Database) ListSnapStoreFilesLocations() ([]SnapStoreFilesLocation, error) {
	return nil, nil
}

// // CreateSnapshot creates a new snapshot object in the database.
// func (d *Database) CreateSnapshot(snapID, vmID string, disks []params.DiskSnapshot) (Snapshot, error) {
// 	snap := Snapshot{
// 		ID:        snapID,
// 		VMID:      vmID,
// 		Disks:     disks,
// 		CreatedAt: time.Now().UTC(),
// 	}
// 	if err := d.con.Save(&snap); err != nil {
// 		return Snapshot{}, errors.Wrap(err, "adding sync folder")
// 	}

// 	return snap, nil
// }

// // DeleteSnapshot removes a snapshot object from the database.
// func (d *Database) DeleteSnapshot(snapID string) error {
// 	var snap Snapshot
// 	if err := d.con.One("ID", snapID, &snap); err != nil {
// 		if err != storm.ErrNotFound {
// 			return errors.Wrap(err, "fetching sync folder")
// 		}
// 		return nil
// 	}

// 	if err := d.con.DeleteStruct(&snap); err != nil {
// 		return errors.Wrap(err, "deleting snapshot")
// 	}

// 	return nil
// }

// // DeleteVMSnapshots deletes all snapshots for a VM.
// func (d *Database) DeleteVMSnapshots(vmID string) error {
// 	if err := d.con.Select(q.Eq("VMID", vmID)).Delete(&Snapshot{}); err != nil {
// 		return errors.Wrap(err, "deleting snapshots")
// 	}
// 	return nil
// }

// // ListSnapshots lists all snapshots for a VM.
// func (d *Database) ListSnapshots(vmID string) ([]Snapshot, error) {
// 	var snaps []Snapshot
// 	if err := d.con.Select(q.Eq("VMID", vmID)).OrderBy("CreatedAt").Find(&snaps); err != nil {
// 		if err == storm.ErrNotFound {
// 			return snaps, nil
// 		}
// 		return snaps, errors.Wrap(err, "fetching VM snapshots")
// 	}

// 	return snaps, nil
// }

// // GetSnapshot gets one snapshot by ID.
// func (d *Database) GetSnapshot(snapID string) (Snapshot, error) {
// 	var snap Snapshot
// 	if err := d.con.One("ID", snapID, &snap); err != nil {
// 		return Snapshot{}, errors.Wrap(err, "fetching snapshot")
// 	}

// 	return snap, nil
// }
