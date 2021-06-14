package db

import (
	"time"

	"github.com/asdine/storm"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

// Open opens the database at path and returns a *bolt.DB object
func Open(path string) (*storm.DB, error) {
	db, err := storm.Open(path, storm.BoltOptions(0600, &bolt.Options{Timeout: 1 * time.Second}))
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
	con      *storm.DB
}

// DBConnection returns the DB connection
func (d *Database) DBConnection() *storm.DB {
	return d.con
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
