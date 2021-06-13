package internal

import (
	"os"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// NewSnapStore creates a new snap store for device.
func NewSnapStore(device string, SnapFilesRootDir string, size uint64) (*SnapStore, error) {
	dev, err := findDeviceByPath(device)
	if err != nil {
		return nil, errors.Wrap(err, "finding device")
	}
	snapDevice, err := getBlockDeviceInfoFromFile(SnapFilesRootDir)
	if err != nil {
		return nil, errors.Wrap(err, "getting block device")
	}

	newUUID := uuid.New()
	uuidAsBytes := [16]byte(newUUID)
	snapStore := &SnapStore{
		ID:     uuidAsBytes,
		Device: dev,
		SnapDevice: DevID{
			Major: snapDevice.major,
			Minor: snapDevice.minor,
		},
		SnapFilesRootDir: SnapFilesRootDir,
	}

	return snapStore, nil
}

type SnapStore struct {
	ID               [16]byte
	Device           DevID
	SnapDevice       DevID
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
