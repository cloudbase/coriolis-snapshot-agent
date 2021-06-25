package manager

import (
	"log"
	"time"

	"coriolis-veeam-bridge/db"
	"coriolis-veeam-bridge/worker/common"

	"github.com/pkg/errors"
)

func (m *Snapshot) Start() error {
	go m.handleWatcherMessages()
	return nil
}

func (m *Snapshot) Stop() error {
	close(m.msgChan)
	return nil
}

func (m *Snapshot) Wait() error {
	select {
	case <-m.watcherMessagesQuit:
		log.Printf("snapstore watcher has stopped")
		return nil
	case <-time.After(20 * time.Second):
		return errors.Errorf("timed out waiting for worker to stop")
	}
}

// In case of any error, we only log. Should we treat errors as critical
// and send it up to the main function where we exit?
func (m *Snapshot) handleWatcherMessages() {
	defer close(m.watcherMessagesQuit)
	for {
		select {
		case msg, ok := <-m.msgChan:
			if !ok {
				// channel was closed. Exiting.
				return
			}
			switch val := msg.(type) {
			case common.SnapStoreAddFileMessage:
				log.Printf("a new file was added to snap store %s (fill status: %d)", val.SnapStoreID.String(), val.FillStatus)
				if err := m.RecordSnapStoreFileInDB(val.SnapStoreID.String(), val.FilePath, val.FileSize); err != nil {
					log.Printf("failed to add snap store %s file to DB: %+v", val.FilePath, err)
				}
			case common.SnapStoreDeletedMessage:
				files, err := m.db.ListSnapStoreFilesForSnapStore(val.SnapStoreID.String())
				if err != nil {
					log.Printf("failed to fetch snap store file list from db")
				} else {
					for _, file := range files {
						if err := m.db.DeleteSnapStoreFile(file.TrackingID); err != nil {
							log.Printf("failed to delete snap store file %s from db: %+v", file.TrackingID, err)
						}
					}
				}
				if err := m.db.DeleteSnapStore(val.SnapStoreID.String()); err != nil {
					log.Printf("failed to delete snapstore %s from db %+v", val.SnapStoreID.String(), err)
				}
			case common.SnapStoreOverflowMessage:
				// The snapshots are now, most likely, invalid. Mark them as invalid in the DB, and delete
				// them from the system.
				volumeSnapshots, err := m.db.ListVolumeSnapshotsBySnapstoreID(val.SnapStoreID.String())
				if err != nil {
					log.Printf("failed to fetch volume snapshots by snap store ID")
					continue
				}
				for _, val := range volumeSnapshots {
					val.Status = db.VolumeStatusOverflow
					if err := m.db.UpdateVolumeSnapshot(val); err != nil {
						log.Printf("failed to update volume snapshot %s in DB", val.TrackingID)
					}
				}
			case common.ErrorMessage:
				// TODO: Do something more meaningful here.
				log.Printf("watcher for snapstore %s encountered an error %+v", val.SnapstoreID.String(), val.Error)
			default:
				log.Printf("got invalid message type: %T", val)
			}
		case <-m.ctx.Done():
			return
		}
	}
}
