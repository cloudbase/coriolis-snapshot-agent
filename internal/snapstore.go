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

package internal

import (
	"coriolis-snapshot-agent/internal/types"
	"coriolis-snapshot-agent/internal/util"
	"os"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// NewSnapStore creates a new snap store for device.
func NewSnapStore(device string, SnapFilesRootDir string, size uint64) (*SnapStore, error) {
	dev, err := util.FindDeviceByPath(device)
	if err != nil {
		return nil, errors.Wrap(err, "finding device")
	}
	snapDevice, err := util.GetBlockDeviceInfoFromFile(SnapFilesRootDir)
	if err != nil {
		return nil, errors.Wrap(err, "getting block device")
	}

	newUUID := uuid.New()
	uuidAsBytes := [16]byte(newUUID)
	snapStore := &SnapStore{
		ID:     uuidAsBytes,
		Device: dev,
		SnapDevice: types.DevID{
			Major: snapDevice.Major,
			Minor: snapDevice.Minor,
		},
		SnapFilesRootDir: SnapFilesRootDir,
	}

	return snapStore, nil
}

type SnapStore struct {
	ID               [16]byte
	Device           types.DevID
	SnapDevice       types.DevID
	SnapFilesRootDir string
	SnapFiles        []string
}

func (s *SnapStore) Init() error {
	if s.SnapFilesRootDir == "" {
		return errors.Errorf("invalid SnapFilesRootDir")
	}

	fileInfo, err := os.Stat(s.SnapFilesRootDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "checking snapshot dir target")
		}

		if err := os.MkdirAll(s.SnapFilesRootDir, 00750); err != nil {
			return errors.Wrap(err, "creating snap store folder")
		}
	} else {
		if !fileInfo.Mode().IsDir() {
			return errors.Errorf("%s exists and is not a folder", s.SnapFilesRootDir)
		}
	}
	return nil
}

func (s *SnapStore) Validate() error {
	// TODO: Validate:
	// * if the block device is part of a device mapper
	//   * make sure snap store device is on a different device
	//   * make sure that the snap store device is also a valid target for a snap store
	return nil
}
