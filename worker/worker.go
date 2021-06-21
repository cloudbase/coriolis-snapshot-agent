package worker

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"coriolis-veeam-bridge/apiserver/params"
	"coriolis-veeam-bridge/manager"

	"github.com/pkg/errors"
)

var (
	// minimum amount of free space in a snap store (2 GB). Any less than this, and we add another
	// 2 GB of space.
	minimumSpaceForStore uint64 = 2 * 1024 * 1024 * 1024
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
	mux        sync.Mutex
}

func (s *SnapStoreTracker) Start() error {
	go s.notifyWorker()
	go s.storageCapacityWatcher()
	if err := s.mgr.PopulateSnapStoreWatcher(); err != nil {
		return errors.Wrap(err, "getting snap stores info")
	}
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
	s.mux.Lock()
	defer s.mux.Unlock()

	// store, err := s.mgr.GetSnapStoreFilesLocation()
	disk, err := s.mgr.GetTrackedDisk(store.TrackedDiskID)
	if err != nil {
		return errors.Wrap(err, "fetching disk")
	}
	snapStore, err := s.mgr.GetSnapStore(store.ID)
	if err != nil {
		return errors.Wrap(err, "fetching store info")
	}

	location, err := s.mgr.GetSnapStoreFilesLocationByID(snapStore.StorageLocationID)
	if err != nil {
		return errors.Wrap(err, "fetching snap store location")
	}

	var allocationSize uint64
	if snapStore.AllocatedDiskSpace == 0 {
		// This is the first chunk we add. Make it max(10% of total disk size, 2GB)
		calculatedSize := float64(disk.Size) * 0.1
		allocationSize = uint64(math.Max(float64(calculatedSize), float64(minimumSpaceForStore)))
		if location.AvailableCapacity < uint64(allocationSize) {
			return errors.Errorf("Cannot allocate %d bytes for snap store %s. Location only has %d bytes available", allocationSize, snapStore.ID, location.AvailableCapacity)
		}
		log.Printf("empty snap store for disk %s. Adding %d bytes to snap store %s", disk.Path, int64(allocationSize), snapStore.ID)
		if err := s.mgr.AddCapacityToSnapStore(snapStore.ID, allocationSize); err != nil {
			return errors.Wrapf(err, "adding capacity to snap store %s", snapStore.ID)
		}
		return nil
	}

	usedPercent := uint64(snapStore.StorageUsage) * 100 / snapStore.AllocatedDiskSpace
	remainingSpace := snapStore.AllocatedDiskSpace - snapStore.StorageUsage

	if remainingSpace < minimumSpaceForStore {
		log.Printf("snap store %s usage is %d out of %d (%d%%)", snapStore.ID, snapStore.StorageUsage, snapStore.AllocatedDiskSpace, usedPercent)
		log.Printf("adding %d bytes to snap store %s", int64(allocationSize), snapStore.ID)
		if err := s.mgr.AddCapacityToSnapStore(snapStore.ID, minimumSpaceForStore); err != nil {
			return errors.Wrapf(err, "adding capacity to snap store %s", snapStore.ID)
		}
	}

	return nil
}

// storageCapacityWatcher is a worker that watches the usage of each snap store and automatically
// adds more storage if needed. If a snap store runs out of storage before we're done with a snapshot,
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
