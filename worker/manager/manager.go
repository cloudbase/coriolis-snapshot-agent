// Copyright 2019 Cloudbase Solutions Srl
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package manager

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/timshannon/bolthold"

	"coriolis-snapshot-agent/apiserver/params"
	"coriolis-snapshot-agent/config"
	"coriolis-snapshot-agent/db"
	vErrors "coriolis-snapshot-agent/errors"
	"coriolis-snapshot-agent/internal/ioctl"
	"coriolis-snapshot-agent/internal/storage"
	"coriolis-snapshot-agent/internal/types"
	"coriolis-snapshot-agent/internal/util"
	"coriolis-snapshot-agent/worker/snapstore"
)

type NotificationType string

var (
	SnapStoreEvent NotificationType = "snapStoreCreate"
)

func NewManager(ctx context.Context, cfg *config.Config, udevMonitor *storage.UdevMonitor) (manager *Snapshot, err error) {
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
		cfg:                              cfg,
		ctx:                              ctx,
		db:                               database,
		notifyChannels:                   map[NotificationType][]chan interface{}{},
		snapStoreCharacterDeviceWatchers: map[string]*snapstore.CharacterDeviceWatcher{},
		msgChan:                          make(chan interface{}, 50),
		udevMonitor:                      udevMonitor,
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

	if err := snapshotMaganer.PopulateSnapStoreWatcher(); err != nil {
		return nil, errors.Wrap(err, "populating watchers")
	}
	return snapshotMaganer, nil
}

type Snapshot struct {
	cfg            *config.Config
	db             *db.Database
	notifyChannels map[NotificationType][]chan interface{}

	// snapStores is a list of snap stores we currently track
	snapStoreCharacterDeviceWatchers map[string]*snapstore.CharacterDeviceWatcher
	// communication channel with snapstore watchers. Messages indicating that
	// a new chunk of data has been added to a snap store, or an overflow
	// event, will be sent back through this channel.
	msgChan chan interface{}

	watcherMessagesQuit chan struct{}
	ctx                 context.Context

	mux         sync.Mutex
	regMux      sync.Mutex
	udevMonitor *storage.UdevMonitor
}

func (m *Snapshot) RecordWatcher(snapstoreID string, watcher *snapstore.CharacterDeviceWatcher) {
	m.regMux.Lock()
	defer m.regMux.Unlock()

	m.snapStoreCharacterDeviceWatchers[snapstoreID] = watcher
}

func (m *Snapshot) RemoveCharacterDeviceWatcher(snapstoreID string) {
	m.regMux.Lock()
	defer m.regMux.Unlock()

	delete(m.snapStoreCharacterDeviceWatchers, snapstoreID)
}

func (m *Snapshot) GetCharacterDeviceWatcher(snapstoreID string) (*snapstore.CharacterDeviceWatcher, error) {
	m.regMux.Lock()
	defer m.regMux.Unlock()

	w, ok := m.snapStoreCharacterDeviceWatchers[snapstoreID]
	if !ok {
		return nil, vErrors.NewNotFoundError("snap store watcher for %s not found", snapstoreID)
	}
	return w, nil
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

// Expose the internal mutex. We need to lock the manager while we download ranges.
// TODO: find a better solution for this.
func (m *Snapshot) Lock() {
	m.mux.Lock()
}

func (m *Snapshot) Unlock() {
	m.mux.Unlock()
}

func (m *Snapshot) listDisks(includeVirtual bool, includeSwap bool) ([]storage.BlockVolume, error) {
	devices, err := storage.BlockDeviceList(false, includeVirtual, includeSwap)
	if err != nil {
		return nil, errors.Wrap(err, "listing devices")
	}

	toExclude := m.cfg.CowDestinationDevices()

	var ret []storage.BlockVolume
	for _, val := range devices {
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

func (m *Snapshot) ListDisks(includeVirtual bool, includeSwap bool) ([]params.BlockVolume, error) {
	devices, err := m.listDisks(includeVirtual, includeSwap)
	if err != nil {
		return nil, errors.Wrap(err, "listing devices")
	}

	ret := make([]params.BlockVolume, len(devices))
	for idx, val := range devices {
		ret[idx] = util.InternalBlockVolumeToParamsBlockVolume(val)
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
	ret := util.InternalBlockVolumeToParamsBlockVolume(volume)
	ret.TrackingID = disk.TrackingID
	return ret, nil
}

func (m *Snapshot) findDiskByPath(path string) (storage.BlockVolume, error) {
	disks, err := m.listDisks(true, true)
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
			SectorSize: uint32(volume.LogicalSectorSize),
		}

		dbObject, err = m.db.CreateTrackedDisk(addDevParams)
		if err != nil {
			return params.BlockVolume{}, errors.Wrapf(err, "adding db entry for %s", volume.Path)
		}
	} else {
		dbObject = exists
	}

	ret := util.InternalBlockVolumeToParamsBlockVolume(volume)
	ret.TrackingID = dbObject.TrackingID
	return ret, nil
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
		StorageLocationID: storeLocation.Path,
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
			StorageLocationID: val.SnapStoreFilesLocation.Path,
		}
	}
	return ret, nil
}

///////////////
// Snapshots //
///////////////

func (m *Snapshot) ensureSnapStoreForDisk(diskID string) (db.SnapStore, error) {
	trackedDisk, err := m.db.GetTrackedDiskByTrackingID(diskID)
	if err != nil {
		return db.SnapStore{}, errors.Wrap(err, "fetching disk info")
	}

	store, err := m.db.GetSnapStoreByDiskID(trackedDisk.TrackingID)
	if err != nil {
		if !errors.Is(err, vErrors.ErrNotFound) {
			return db.SnapStore{}, errors.Wrap(err, "fetching snap stores")
		}
	} else {
		return store, nil
	}

	newStore, err := m.CreateSnapStore(diskID)
	if err != nil {
		return db.SnapStore{}, errors.Wrap(err, "creating snap store")
	}

	watcher, err := m.GetCharacterDeviceWatcher(newStore.SnapStoreID)
	if err != nil {
		return db.SnapStore{}, errors.Wrap(err, "fetching snapstore watcher")
	}
	// Allocates 20% of device size.
	filePath, size, err := watcher.AllocateInitialStorage()
	if err != nil {
		return db.SnapStore{}, errors.Wrap(err, "allocating disk space")
	}

	if err := m.RecordSnapStoreFileInDB(newStore.SnapStoreID, filePath, size); err != nil {
		return db.SnapStore{}, errors.Wrap(err, "recording file in DB")
	}
	return newStore, nil
}

func filterCBTInfo(info []types.CBTInfo, dev types.DevID) (types.CBTInfo, error) {
	for _, val := range info {
		if val.DevID.Major == dev.Major && val.DevID.Minor == dev.Minor {
			return val, nil
		}
	}
	return types.CBTInfo{}, vErrors.NewNotFoundError("could not find CBT info for device %d:%d", dev.Major, dev.Minor)
}

func (m *Snapshot) cleanupSnapStore(snapStore db.SnapStore) error {
	storeID, parseErr := uuid.Parse(snapStore.SnapStoreID)
	if parseErr != nil {
		return errors.Wrap(parseErr, "parsing")
	}
	storeParams := types.SnapStore{
		ID: [16]byte(storeID),
	}
	cleanupRet, cleanupErr := ioctl.SnapStoreCleanup(storeParams)
	if cleanupErr != nil {
		return errors.Wrap(cleanupErr, "cleanup ioctl")
	}
	if cleanupRet.FilledBytes == ioctl.SNAP_STORE_NOT_FOUND {
		return errors.Errorf("Snap store %s is already gone", snapStore.SnapStoreID)
	}

	return nil
}

// CreateSnapshot creates a new snapshot of one or more disks.
func (m *Snapshot) CreateSnapshot(param params.CreateSnapshotRequest) (params.SnapshotResponse, error) {
	m.mux.Lock()
	defer m.mux.Unlock()
	var err error

	// Taking multiple snapshots of the same disk seems to be unstable. Limit to one active
	// snapshot per disk.
	for _, disk := range param.TrackedDiskIDs {
		snap, err := m.db.ListSnapshotsForDisk(disk)
		if err != nil {
			return params.SnapshotResponse{}, errors.Wrap(err, "listing snapshot")
		}
		if len(snap) > 0 {
			return params.SnapshotResponse{}, vErrors.NewConflictError("disk %s already has a snapshot", disk)
		}
	}

	// Ensure snap stores
	var devices []types.DevID
	trackedDiskMap := map[types.DevID]db.TrackedDisk{}
	snapStoreMap := map[types.DevID]db.SnapStore{}
	var snapStores []db.SnapStore
	for _, disk := range param.TrackedDiskIDs {
		store, err := m.ensureSnapStoreForDisk(disk)
		if err != nil {
			return params.SnapshotResponse{}, errors.Wrap(err, "creating snap store")
		}
		snapStores = append(snapStores, store)

		dbDev, err := m.db.GetTrackedDiskByTrackingID(disk)
		if err != nil {
			return params.SnapshotResponse{}, errors.Wrap(err, "getting device")
		}
		newDev := types.DevID{
			Major: dbDev.Major,
			Minor: dbDev.Minor,
		}
		devices = append(devices, newDev)
		trackedDiskMap[newDev] = dbDev
		snapStoreMap[newDev] = store
	}

	// Cleanup snap stores in case of error
	defer func() {
		if err != nil {
			for _, val := range snapStores {
				if cleanupErr := m.cleanupSnapStore(val); cleanupErr != nil {
					log.Printf("cleaning up snap store: %+v", cleanupErr)
				}
			}
		}
	}()

	// Gather info before snapshot
	cbtInfoPreSnap, err := ioctl.GetCBTInfo()
	if err != nil {
		return params.SnapshotResponse{}, errors.Wrap(err, "fetching CBT info")
	}

	imgsPreSnap, err := ioctl.CollectSnapshotImages()
	if err != nil {
		return params.SnapshotResponse{}, errors.Wrap(err, "collecting images")
	}

	// Create snapshot
	snapshot, err := ioctl.CreateSnapshot(devices)
	if err != nil {
		return params.SnapshotResponse{}, errors.Wrap(err, "creating snapshot")
	}

	// cleanup func in case of error
	defer func() {
		if err != nil {
			ioctl.DeleteSnapshot(snapshot.SnapshotID)
		}
	}()

	// Gather info post-snap
	cbtInfoPostSnap, err := ioctl.GetCBTInfo()
	if err != nil {
		return params.SnapshotResponse{}, errors.Wrap(err, "fetching CBT info")
	}

	imgsPostSnap, err := ioctl.CollectSnapshotImages()
	if err != nil {
		return params.SnapshotResponse{}, errors.Wrap(err, "collecting images")
	}

	// create DB objects
	newSnapshotParams := db.Snapshot{
		SnapshotID: fmt.Sprintf("%d", snapshot.SnapshotID),
	}
	var newVolumeSnapshots []db.VolumeSnapshot
	for _, dev := range devices {
		dbDev := trackedDiskMap[dev]
		volSnapID := uuid.New()
		volumeSnapshot := db.VolumeSnapshot{
			TrackingID:     volSnapID.String(),
			SnapshotID:     fmt.Sprintf("%d", snapshot.SnapshotID),
			OriginalDevice: dbDev,
			Status:         db.VolumeStatusHealthy,
		}

		cbtInfoNew, err := filterCBTInfo(cbtInfoPostSnap, dev)
		if err != nil {
			return params.SnapshotResponse{}, errors.Wrap(err, "finding cbt info")
		}
		cbtInfoOld, err := filterCBTInfo(cbtInfoPreSnap, dev)
		if err != nil {
			return params.SnapshotResponse{}, errors.Wrap(err, "finding cbt info")
		}

		diff := int(cbtInfoNew.SnapNumber) - int(cbtInfoOld.SnapNumber)
		if diff != 1 {
			// Note (gsamfira): Neither the snap number, nor the image info get returned by the
			// create snapshot ioctl, so we have to resort to hacks like this to find out
			// what new resources we have after taking a snapshot. I still hope that there is
			// an easier way to get this info, and it's just my ignorance that lead to this mess.
			// Will investigate later.
			return params.SnapshotResponse{}, errors.Errorf("failed to determine proper CBT info for device: %d:%d", dev.Major, dev.Minor)
		}
		volumeSnapshot.SnapshotNumber = uint32(cbtInfoNew.SnapNumber)
		volumeSnapshot.GenerationID = uuid.UUID(cbtInfoNew.GenerationID).String()

		var images []types.ImageInfo
		for _, imagePost := range imgsPostSnap.ImageInfo {
			if imagePost.OriginalDevID == dev {
				found := false
				for _, imagePre := range imgsPreSnap.ImageInfo {
					if imagePre.OriginalDevID == dev && imagePost.SnapshotDevID == imagePre.SnapshotDevID {
						found = true
						break
					}
				}
				if !found {
					images = append(images, imagePost)
				}
			}
		}
		if len(images) != 1 {
			// something else created a snapshot while we were processing this function.
			return params.SnapshotResponse{}, errors.Errorf("expected to find 1 new image, found %d", len(images))
		}

		imageMajor := images[0].SnapshotDevID.Major
		imageMinor := images[0].SnapshotDevID.Minor
		devFromID, err := m.udevMonitor.GetUdevDevice(int(imageMajor), int(imageMinor))
		if err != nil {
			return params.SnapshotResponse{}, errors.Errorf("failed to udev detect image device by ID (%d:%d): %+v", imageMajor, imageMinor, err)
		}
		if devFromID.DeviceStatus != storage.DeviceStatusActive {
			log.Printf("WARNING! Status of device with ID %d:%d is not %s. Actual device status: %s\n", imageMajor, imageMinor, storage.DeviceStatusActive, devFromID.DeviceStatus)
		}

		// Record resources in the database
		snapImageID := uuid.New()
		newSnapImage := db.SnapshotImage{
			TrackingID: snapImageID.String(),
			DevicePath: devFromID.DeviceNode,
			Major:      images[0].SnapshotDevID.Major,
			Minor:      images[0].SnapshotDevID.Minor,
		}

		if _, err := m.db.CreateSnapshotImage(newSnapImage); err != nil {
			return params.SnapshotResponse{}, errors.Wrap(err, "creating snapshot image")
		}
		defer func(snapImage db.SnapshotImage) {
			if err != nil {
				log.Printf("deleting snapshot image %s", snapImage.TrackingID)
				if imgDeleteErr := m.db.DeleteSnapshotImage(snapImage.TrackingID); imgDeleteErr != nil {
					log.Printf("failed to delete snapshot image fro db: %q", imgDeleteErr)
				}
			}
		}(newSnapImage)

		volumeSnapshot.SnapshotImage = newSnapImage
		if _, err := m.db.CreateVolumeSnapshot(volumeSnapshot); err != nil {
			return params.SnapshotResponse{}, errors.Wrap(err, "creating volume snapshot")
		}

		defer func(volSnap db.VolumeSnapshot) {
			if err != nil {
				log.Printf("cleaning up volume snapshot %s", volSnap.TrackingID)
				if volErr := m.db.DeleteVolumeSnapshot(volSnap.TrackingID); volErr != nil {
					log.Printf("error deleting volume snapshot %s from database: %q", volSnap.TrackingID, volErr)
				}
			}
		}(volumeSnapshot)

		bitmap, err := ioctl.GetCBTBitmap(dev)
		if err != nil {
			return params.SnapshotResponse{}, errors.Wrapf(err, "fetchinb bitmap for device %d:%d", dev.Major, dev.Minor)
		}
		volumeSnapshot.Bitmap = bitmap.Buff
		volumeSnapshot.SnapStore = snapStoreMap[dev]
		newVolumeSnapshots = append(newVolumeSnapshots, volumeSnapshot)
	}

	newSnapshotParams.VolumeSnapshots = newVolumeSnapshots

	newSnapStore, err := m.db.CreateSnapshot(newSnapshotParams)
	if err != nil {
		return params.SnapshotResponse{}, errors.Wrap(err, "crating snapshot in DB")
	}
	return internalSnapToSnapResponse(newSnapStore), nil
}

func (m *Snapshot) DeleteSnapshot(snapshotID string) error {
	m.mux.Lock()
	defer m.mux.Unlock()

	snapshot, err := m.db.GetSnapshot(snapshotID)
	if err != nil {
		if !errors.Is(err, vErrors.ErrNotFound) {
			return errors.Wrap(err, "fetching snapshot from DB")
		}
		log.Printf("Could not find snapshot with id: %s --> %+v", snapshotID, err)
		return nil
	}

	parseSnapshotID, err := strconv.ParseUint(snapshotID, 10, 64)
	if err != nil {
		return errors.Wrap(err, "parsing snapshot ID")
	}
	if err := ioctl.DeleteSnapshot(parseSnapshotID); err != nil {
		return errors.Wrap(err, "removing snapshot")
	}

	for _, vol := range snapshot.VolumeSnapshots {
		snapStoreUUID, err := uuid.Parse(vol.SnapStore.SnapStoreID)
		if err != nil {
			return errors.Wrap(err, "parsing snap store ID")
		}
		snapStoreInternal := types.SnapStore{
			ID: [16]byte(snapStoreUUID),
		}

		_, err = ioctl.SnapStoreCleanup(snapStoreInternal)
		if err != nil {
			return errors.Wrap(err, "cleaning snap store")
		}

		files, err := m.db.ListSnapStoreFilesForSnapStore(vol.SnapStore.SnapStoreID)
		if err != nil {
			return errors.Wrap(err, "fetching store files")
		}

		for _, file := range files {
			if err := m.db.DeleteSnapStoreFile(file.TrackingID); err != nil {
				log.Printf("failed to delete snap store file %s from db", file.TrackingID)
			}
		}

		if err := m.db.DeleteSnapStore(vol.SnapStore.SnapStoreID); err != nil {
			return errors.Wrap(err, "deleting snap store")
		}

	}
	if err := m.db.DeleteSnapshot(snapshotID); err != nil {
		log.Printf("removing snapshot %s from DB", snapshotID)
		if !errors.Is(err, vErrors.ErrNotFound) {
			return errors.Wrapf(err, "removing snapshot %s from DB", snapshotID)
		}
		log.Printf("snapshot %s not in DB", snapshotID)
	}
	return nil
}

func (m *Snapshot) GetSnapshot(snapshotID string) (params.SnapshotResponse, error) {
	snapshot, err := m.db.GetSnapshot(snapshotID)
	if err != nil {
		return params.SnapshotResponse{}, errors.Wrap(err, "fetching snapshot from DB")
	}
	return internalSnapToSnapResponse(snapshot), nil
}

func (m *Snapshot) FindVolumeSnapshotForDisk(snapshotID string, diskTrackingID string) (db.VolumeSnapshot, error) {
	snap, err := m.db.GetSnapshot(snapshotID)
	if err != nil {
		return db.VolumeSnapshot{}, errors.Wrap(err, "fetching snapshot from DB")
	}

	var volSnap db.VolumeSnapshot
	for _, val := range snap.VolumeSnapshots {
		if val.OriginalDevice.TrackingID == diskTrackingID {
			volSnap = val
		}
	}
	if volSnap.OriginalDevice.TrackingID == "" {
		return db.VolumeSnapshot{}, vErrors.NewNotFoundError("could not find volume snapshot for %s", diskTrackingID)
	}
	return volSnap, nil
}

func (m *Snapshot) ListSnapshots() ([]params.SnapshotResponse, error) {
	snapshots, err := m.db.ListAllSnapshots()
	if err != nil {
		return []params.SnapshotResponse{}, errors.Wrap(err, "listing db snapshots")
	}

	ret := make([]params.SnapshotResponse, len(snapshots))

	for idx, snap := range snapshots {
		ret[idx] = internalSnapToSnapResponse(snap)
	}
	return ret, nil
}

// Snapshot consumption

func (m *Snapshot) fetchIncrements(bitmap []byte, prevNumber, currentNumber int, cbtBlkSize int) []params.DiskRange {
	var ranges []params.DiskRange
	var tmpStartSector int
	var tmpLength int
	for i := 0; i < len(bitmap); i++ {
		if prevNumber != 0 {
			if int(bitmap[i]) <= prevNumber || int(bitmap[i]) > currentNumber {
				continue
			}
			if tmpStartSector == 0 {
				tmpStartSector = i
				tmpLength = int(cbtBlkSize)
				continue
			}
		}

		if tmpStartSector*cbtBlkSize+tmpLength != i*cbtBlkSize {
			ranges = append(ranges, params.DiskRange{
				StartOffset: uint64(tmpStartSector * cbtBlkSize),
				Length:      uint64(tmpLength),
			})
			tmpStartSector = i
			tmpLength = cbtBlkSize
			continue
		}
		tmpLength += cbtBlkSize
	}

	ranges = append(ranges, params.DiskRange{
		StartOffset: uint64(tmpStartSector * cbtBlkSize),
		Length:      uint64(tmpLength),
	})
	return ranges
}

func (m *Snapshot) GetChangedSectors(currentSnapshotID string, trackedDiskID string, previousGenerationID string, previousNumber uint32) (params.ChangesResponse, error) {
	if previousGenerationID != "" {
		if _, err := uuid.Parse(previousGenerationID); err != nil {
			return params.ChangesResponse{}, errors.Wrap(err, "parsing generation ID")
		}
	}
	volumeSnapshot, err := m.FindVolumeSnapshotForDisk(currentSnapshotID, trackedDiskID)
	if err != nil {
		return params.ChangesResponse{}, errors.Wrap(err, "finding volume snapshot")
	}

	var backupType params.BackupType = params.BackupTypeIncremental
	if previousNumber == 0 || previousGenerationID != volumeSnapshot.GenerationID {
		backupType = params.BackupTypeFull
		// gets full disk
		previousNumber = 0
	}

	cbtBlkSize, err := ioctl.GetTrackingBlockSize()
	if err != nil {
		return params.ChangesResponse{}, errors.Wrap(err, "fetching CBT block size")
	}

	ranges := m.fetchIncrements(volumeSnapshot.Bitmap, int(previousNumber), int(volumeSnapshot.SnapshotNumber), int(cbtBlkSize))
	if err != nil {
		return params.ChangesResponse{}, errors.Wrap(err, "fetching ranges")
	}
	return params.ChangesResponse{
		TrackedDiskID: trackedDiskID,
		SnapshotID:    currentSnapshotID,
		BackupType:    backupType,
		CBTBlockSize:  int(cbtBlkSize),
		Ranges:        ranges,
	}, nil
}
