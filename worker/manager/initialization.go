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
	"coriolis-snapshot-agent/apiserver/params"
	vErrors "coriolis-snapshot-agent/errors"
	"coriolis-snapshot-agent/internal/types"
	"coriolis-snapshot-agent/worker/common"
	"coriolis-snapshot-agent/worker/snapstore"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

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
				log.Printf("device %d:%d is already tracked", major, minor)
				return true
			}
		}
	}
	return false
}

func (m *Snapshot) initStorageMappings() error {
	if !m.cfg.AutoInitPhysicalDisks {
		return nil
	}

	for _, mapping := range m.cfg.SnapStoreMappings {
		param := params.CreateSnapStoreMappingRequest{
			SnapStoreLocation: mapping.Location,
			TrackedDisk:       mapping.Device,
		}
		if _, err := m.CreateSnapStoreMapping(param); err != nil {
			if !errors.Is(err, &vErrors.ConflictError{}) {
				return errors.Wrap(err, "init mappings")
			}
		}
	}

	return nil
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
	if err := m.initStorageMappings(); err != nil {
		return errors.Wrap(err, "adding storage mappings")
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

func (m *Snapshot) PopulateSnapStoreWatcher() error {
	stores, err := m.db.ListSnapStores()
	if err != nil {
		return errors.Wrap(err, "initializing snap storage worker")
	}

	for _, store := range stores {
		storeID, err := uuid.Parse(store.SnapStoreID)
		if err != nil {
			return errors.Wrap(err, "parsing uuid")
		}
		deviceID := types.DevID{
			Major: store.TrackedDisk.Major,
			Minor: store.TrackedDisk.Minor,
		}

		snapDisk := types.DevID{
			Major: store.StorageLocation.Major,
			Minor: store.StorageLocation.Minor,
		}
		snapCharacterDeviceWatcherParams := common.CreateSnapStoreParams{
			ID:                [16]byte(storeID),
			BaseDir:           store.Path(),
			SnapDeviceID:      snapDisk,
			DeviceID:          deviceID,
			SnapStoreFileSize: m.cfg.SnapStoreFileSize,
		}
		snapCharacterDeviceWatcher, err := snapstore.NewSnapStoreCharacterDeviceWatcher(snapCharacterDeviceWatcherParams, m.msgChan)
		if err != nil {
			return errors.Wrap(err, "creating snap store")
		}
		m.RecordWatcher(store.SnapStoreID, snapCharacterDeviceWatcher)
	}
	return nil
}
