package manager

import (
	"coriolis-veeam-bridge/apiserver/params"
	"coriolis-veeam-bridge/config"
	"coriolis-veeam-bridge/internal/storage"
	"sync"

	"github.com/pkg/errors"
)

func NewManager(cfg *config.Config) (*Snapshot, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Snapshot{
		cfg: cfg,
	}, nil
}

type Snapshot struct {
	cfg *config.Config
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

func (m *Snapshot) ListDisks(includeVirtual bool) ([]params.BlockVolume, error) {
	devices, err := storage.BlockDeviceList(false)
	if err != nil {
		return nil, errors.Wrap(err, "listing devices")
	}

	var ret []params.BlockVolume
	for _, val := range devices {
		if !includeVirtual && val.IsVirtual {
			continue
		}
		ret = append(ret, internalBlockVolumeToParamsBlockVolume(val))
	}
	return ret, nil
}
