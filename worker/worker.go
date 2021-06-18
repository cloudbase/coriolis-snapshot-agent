package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"coriolis-veeam-bridge/apiserver/params"
	"coriolis-veeam-bridge/manager"
)

func NewSnapStorageTracker(ctx context.Context, mgr *manager.Snapshot) (*SnapStoreTracker, error) {
	notifyChan := make(chan interface{}, 10)
	tracker := &SnapStoreTracker{
		ctx:               ctx,
		mgr:               mgr,
		notify:            notifyChan,
		watcherWorkerQuit: make(chan struct{}),
		notifyWorkerQuit:  make(chan struct{}),
	}
	mgr.RegisterNotificationChannel(manager.SnapStoreCreateEvent, notifyChan)
	return tracker, nil
}

type SnapStoreTracker struct {
	ctx               context.Context
	mgr               *manager.Snapshot
	notify            chan interface{}
	watcherWorkerQuit chan struct{}
	notifyWorkerQuit  chan struct{}

	snapStores []params.SnapStoreResponse
}

func (s *SnapStoreTracker) Start() error {
	go s.notifyWorker()
	go s.storageCapacityWatcher()
	return nil
}

func (s *SnapStoreTracker) Wait() error {
	ch := make(chan struct{})
	go func() {
		<-s.notifyWorkerQuit
		<-s.watcherWorkerQuit
		close(ch)
	}()
	select {
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for quit")
	case <-ch:
		log.Print("snap store maintenance worker was stopped")
		return nil
	}
}

func (s *SnapStoreTracker) ensureStorageForSnapStore(store params.SnapStoreResponse) error {
	// store, err := s.mgr.GetSnapStoreFilesLocation()
	return nil
}

// storageCapacityWatcher is a worker that watches the usage of each snap store and automatically
// adds more storage if needed. If a snap store runs out of storage, before we're done with a snapshot
// all snapshots that use that snap store will become corrupt.
func (s *SnapStoreTracker) storageCapacityWatcher() {
	log.Printf("Starting snap storage usage watcher")
	ticker := time.NewTicker(time.Duration(5) * time.Second)
	defer close(s.watcherWorkerQuit)
	defer func() {
		ticker.Stop()
	}()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			for _, val := range s.snapStores {
				if err := s.ensureStorageForSnapStore(val); err != nil {
					log.Printf("failed to add storage to snap store %s: %q", val.ID, err)
				}
			}
		}
	}
}

func (s *SnapStoreTracker) notifyWorker() {
	log.Printf("Starting snap storage notify watcher")
	defer close(s.notifyWorkerQuit)
	for {
		select {
		case <-s.ctx.Done():
			return
		case msg, ok := <-s.notify:
			if !ok {
				return
			}
			switch val := msg.(type) {
			case params.SnapStoreResponse:
				found := false
				for _, store := range s.snapStores {
					if val.ID == store.ID {
						found = true
						break
					}
				}
				if found {
					continue
				}
				// Add initial storage chunk
				if err := s.ensureStorageForSnapStore(val); err != nil {
					log.Printf("failed to add storage to snap store %s: %q", val.ID, err)
				}
				s.snapStores = append(s.snapStores, val)
			default:
				log.Printf("got invalid payload of type %T", val)
			}
		}
	}
}
