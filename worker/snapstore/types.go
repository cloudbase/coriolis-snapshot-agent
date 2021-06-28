package snapstore

import (
	"coriolis-snapshot-agent/internal/types"
	"encoding/binary"

	"github.com/pkg/errors"
)

const (
	CHARCMD_UNDEFINED   = 0x00
	CHARCMD_ACKNOWLEDGE = 0x01
	CHARCMD_INVALID     = 0xFF

	CHARCMD_INITIATE              = 0x21
	CHARCMD_NEXT_PORTION          = 0x22
	CHARCMD_NEXT_PORTION_MULTIDEV = 0x23

	CHARCMD_HALFFILL  = 0x41
	CHARCMD_OVERFLOW  = 0x42
	CHARCMD_TERMINATE = 0x43
)

type SnapStoreStretchInitiateParams struct {
	ID [16]byte

	// Empty limit is the number of empty bytes after which
	// the module lets us know we need to add more space to the snap store.
	EmptyLimit        uint64
	SnapStoreDeviceID types.DevID
	Count             uint32
	DeviceIDs         []types.DevID
}

func (s *SnapStoreStretchInitiateParams) Serialize() []byte {
	// 40 is the size of all fields, except for deviceIDs, which is a slice
	// of device ids (8 bytes each)
	arrLen := 40 + 8*len(s.DeviceIDs)
	ret := make([]byte, arrLen)

	processed := 0
	binary.LittleEndian.PutUint32(ret[processed:], CHARCMD_INITIATE)
	processed += 4

	copy(ret[processed:], s.ID[:])
	processed += 16

	binary.LittleEndian.PutUint64(ret[processed:], s.EmptyLimit)
	processed += 8

	binary.LittleEndian.PutUint32(ret[processed:], s.SnapStoreDeviceID.Major)
	processed += 4

	binary.LittleEndian.PutUint32(ret[processed:], s.SnapStoreDeviceID.Minor)
	processed += 4

	binary.LittleEndian.PutUint32(ret[processed:], s.Count)
	processed += 4

	for _, val := range s.DeviceIDs {
		binary.LittleEndian.PutUint32(ret[processed:], val.Major)
		processed += 4

		binary.LittleEndian.PutUint32(ret[processed:], val.Minor)
		processed += 4
	}
	return ret
}

type NextPortionParams struct {
	ID     [16]byte
	Count  uint32
	Ranges []types.Range
}

func (n *NextPortionParams) Serialize() []byte {
	// command - 4 bytes
	// snap store uuid - 16 bytes
	// range count - 4 bytes
	// length of types.Ranges - 16 (two uint64 fields)
	arrLen := 24 + 16*len(n.Ranges)
	ret := make([]byte, arrLen)

	processed := 0
	binary.LittleEndian.PutUint32(ret[processed:], CHARCMD_NEXT_PORTION)
	processed += 4

	copy(ret[processed:], n.ID[:])
	processed += 16

	binary.LittleEndian.PutUint32(ret[processed:], n.Count)
	processed += 4

	for _, val := range n.Ranges {
		binary.LittleEndian.PutUint64(ret[processed:], val.Left)
		processed += 8

		binary.LittleEndian.PutUint64(ret[processed:], val.Right)
		processed += 8
	}

	return ret
}

type NextPortionMultidevParams struct {
	ID                [16]byte
	SnapStoreDeviceID types.DevID
	Count             uint32
	Ranges            []types.Range
}

func (n *NextPortionMultidevParams) Serialize() ([]byte, error) {
	if int(n.Count) != len(n.Ranges) {
		return nil, errors.Errorf("missmatch between count and array length")
	}

	// command - 4 bytes
	// snap store uuid - 16 bytes
	// snap store device - 8 bytes
	// range count - 4 bytes
	// length of types.Ranges - 16 (two uint64 fields)
	arrLen := 32 + 16*len(n.Ranges)
	ret := make([]byte, arrLen)

	processed := 0
	// Command
	binary.LittleEndian.PutUint32(ret[processed:], CHARCMD_INITIATE)
	processed += 4

	// Snapstore ID
	copy(ret[processed:], n.ID[:])
	processed += 16

	// Snap store device ID
	binary.LittleEndian.PutUint32(ret[processed:], n.SnapStoreDeviceID.Major)
	processed += 4

	binary.LittleEndian.PutUint32(ret[processed:], n.SnapStoreDeviceID.Minor)
	processed += 4

	// Ranges count
	binary.LittleEndian.PutUint32(ret[processed:], n.Count)
	processed += 4

	// Ranges buffer
	for _, val := range n.Ranges {
		binary.LittleEndian.PutUint64(ret[processed:], val.Left)
		processed += 8

		binary.LittleEndian.PutUint64(ret[processed:], val.Right)
		processed += 8
	}

	return ret, nil
}
