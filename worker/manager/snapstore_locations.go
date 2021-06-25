package manager

import (
	"coriolis-veeam-bridge/apiserver/params"
	"coriolis-veeam-bridge/db"
	vErrors "coriolis-veeam-bridge/errors"
	"coriolis-veeam-bridge/internal/types"
	"coriolis-veeam-bridge/internal/util"

	"github.com/pkg/errors"
)

/////////////////////////
// Snap store location //
/////////////////////////

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

	files, err := m.db.FindSnapStoreLocationFiles(createdStore.Path)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching snap store files")
	}
	var totalAllocated uint64
	for _, file := range files {
		totalAllocated += file.Size
	}

	return params.SnapStoreLocation{
		// ID:                createdStore.TrackingID,
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

	files, err := m.db.FindSnapStoreLocationFiles(location.Path)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching snap store files")
	}
	var totalAllocated uint64
	for _, file := range files {
		totalAllocated += file.Size
	}

	return params.SnapStoreLocation{
		// ID:                location.TrackingID,
		AllocatedCapacity: totalAllocated,
		AvailableCapacity: fsInfo.BlocksAvailable * uint64(fsInfo.BlockSize),
		TotalCapacity:     location.TotalCapacity,
		Path:              location.Path,
		DevicePath:        location.DevicePath,
		Major:             location.Major,
		Minor:             location.Minor,
	}, nil
}

func (m *Snapshot) GetSnapStoreLocation(path string) (params.SnapStoreLocation, error) {
	dbSnapFileDestination, err := m.db.GetSnapStoreFilesLocation(path)
	if err != nil {
		return params.SnapStoreLocation{}, errors.Wrap(err, "fetching snap store location info")
	}

	return m.getSnapStoreLoctionInfo(dbSnapFileDestination)
}

func (m *Snapshot) GetSnapStoreLocationByID(locationID string) (params.SnapStoreLocation, error) {
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

		files, err := m.db.FindSnapStoreLocationFiles(val.Path)
		if err != nil {
			return []params.SnapStoreLocation{}, errors.Wrap(err, "fetching snap store files")
		}
		var totalAllocated uint64
		for _, file := range files {
			totalAllocated += file.Size
		}

		ret[idx] = params.SnapStoreLocation{
			// ID:                val.TrackingID,
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
