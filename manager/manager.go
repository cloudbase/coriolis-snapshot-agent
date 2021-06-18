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

	"github.com/pkg/errors"

	"coriolis-veeam-bridge/apiserver/params"
	"coriolis-veeam-bridge/config"
	"coriolis-veeam-bridge/db"
	vErrors "coriolis-veeam-bridge/errors"
	"coriolis-veeam-bridge/internal/storage"
	"coriolis-veeam-bridge/internal/types"
	"coriolis-veeam-bridge/internal/util"
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
		cfg: cfg,
		db:  database,
	}
	if dbNeedsInit {
		defer func() {
			// The database requires init, but we failed to initialize
			// state on first run. Delete the newly created DB file, which
			// is not yet properly set up, due to an error returned by one
			// of the bellow functions.
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
	cfg *config.Config
	db  *db.Database
	mux sync.Mutex
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
	}
	return ret, nil
}

// AddSnapStoreLocation creates a new snap store location. Locations hosted on a device
// that is currently tracked, will err out.
func (m *Snapshot) AddSnapStoreLocation(path string) (params.SnapStoreLocation, error) {
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

	return params.SnapStoreLocation{
		AllocatedCapacity: createdStore.AllocatedSize,
		AvailableCapacity: fsInfo.BlocksAvailable * uint64(fsInfo.BlockSize),
		TotalCapacity:     createdStore.TotalCapacity,
		Path:              createdStore.Path,
		DevicePath:        createdStore.DevicePath,
		Major:             createdStore.Major,
		Minor:             createdStore.Minor,
	}, nil
}

func (m *Snapshot) GetSnapStoreFileLocation(path string) (params.SnapStoreLocation, error) {
	dbSnapFileDestination, err := m.db.GetSnapStoreFilesLocation(path)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching snap store location info")
	}

	fsInfo, err := util.GetFileSystemInfoFromPath(path)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching filesystem info")
	}

	return params.SnapStoreLocation{
		AllocatedCapacity: dbSnapFileDestination.AllocatedSize,
		AvailableCapacity: fsInfo.BlocksAvailable * uint64(fsInfo.BlockSize),
		TotalCapacity:     dbSnapFileDestination.TotalCapacity,
		Path:              dbSnapFileDestination.Path,
		DevicePath:        dbSnapFileDestination.DevicePath,
		Major:             dbSnapFileDestination.Major,
		Minor:             dbSnapFileDestination.Minor,
	}, nil
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

		ret[idx] = params.SnapStoreLocation{
			AllocatedCapacity: val.AllocatedSize,
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
	m.mux.Lock()
	defer m.mux.Unlock()

	// cbtInfo, err := ioctl.GetCBTInfo()
	// if err != nil {
	// 	return errors.Wrap(err, "fetching CBT info")
	// }

	// listDisks excludes disks configured as snap store destinations.
	disks, err := m.listDisks(false)
	if err != nil {
		return errors.Wrap(err, "fetching disks list")
	}

	for _, val := range disks {
		log.Printf("checking disk %s\n", val.Path)
		// devID := types.DevID{
		// 	Major: val.Major,
		// 	Minor: val.Minor,
		// }

		_, err = m.db.GetTrackedDisk(val.Major, val.Minor)
		if err != nil {
			if !errors.Is(err, &vErrors.NotFoundError{}) {
				return errors.Wrap(err, "checking tracked disk status")
			}
		}

		// if !deviceIsTracked(val.Major, val.Minor, cbtInfo) {
		// 	log.Printf("Adding %s to tracking", val.Path)
		// 	if err := ioctl.AddDeviceToTracking(devID); err != nil {
		// 		log.Printf("error adding %s to tracking: %s", val.Path, err)
		// 		return errors.Wrapf(err, "adding %s to tracking", val.Path)
		// 	}
		// }

		addDevParams := db.TrackedDisk{
			Path:  val.Path,
			Major: val.Major,
			Minor: val.Minor,
		}

		if _, err := m.db.CreateTrackedDisk(addDevParams); err != nil {
			return errors.Wrapf(err, "adding db entry for %s", val.Path)
		}
	}
	return nil
}

// addSnapStoreFilesLocations adds all configured CoWDestination members
// to the database.
func (m *Snapshot) addSnapStoreFilesLocations() error {
	for _, val := range m.cfg.CoWDestination {
		if _, err := m.AddSnapStoreLocation(val); err != nil {
			if !errors.Is(err, &vErrors.ConflictError{}) {
				return errors.Wrap(err, "creating snap store location")
			}
		}
	}
	return nil
}

// End init functions.
