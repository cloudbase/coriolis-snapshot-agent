package common

import "coriolis-veeam-bridge/internal/types"

type CreateSnapStoreParams struct {
	ID                [16]byte
	BaseDir           string
	SnapDeviceID      types.DevID
	DeviceID          types.DevID
	SnapStoreFileSize uint64
}
