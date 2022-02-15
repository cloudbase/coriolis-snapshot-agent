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

package types

type DevID struct {
	Major uint32
	Minor uint32
}

type CBTInfo struct {
	DevID        DevID
	DevCapacity  uint64
	CBTMapSize   uint32
	SnapNumber   byte
	GenerationID [16]byte
}

type TrackingCollect struct {
	Count   uint32
	CBTInfo *CBTInfo
}

type TrackingReadCBTBitmap struct {
	DevID  DevID
	Offset uint32
	Length uint32
	Buff   []byte
}

type BlockRange struct {
	// Offset in sectors
	Offset uint64
	// Count in sectors
	Count uint64
}

type TrackingMarkDirtyBlocks struct {
	DevID      DevID
	Count      uint32
	BlockRange []BlockRange
}

// Snapshots

type Snapshot struct {
	SnapshotID uint64
	Count      uint32
	DevID      []DevID
}

type SnapshotErrno struct {
	DevID     DevID
	ErrorCode int32
}

type Range struct {
	Left  uint64
	Right uint64
}

// Snap store

type SnapStore struct {
	ID               [16]byte
	SnapshotDeviceID DevID
	Count            uint32
	DevID            []DevID
}

type SnapStoreMemoryLimit struct {
	ID   [16]byte
	Size uint64
}

type SnapStoreFileAdd struct {
	ID         [16]byte
	RangeCount uint32
	// Ranges is a pointer to a list(?) of []Range{}
	Ranges []Range
}

type SnapStoreFileAddMultidev struct {
	ID         [16]byte
	DevID      DevID
	RangeCount uint32
	Ranges     []Range
}

type SnapStoreCleanupParams struct {
	ID          [16]byte
	FilledBytes uint64
}

// collect snapshot images
type ImageInfo struct {
	OriginalDevID DevID
	SnapshotDevID DevID
}

type SnapshotImages struct {
	Count     uint32
	ImageInfo []ImageInfo
}

// collect snapshot data location
type SnapshotdataLocationStart struct {
	DevID       DevID
	MagicLength int32
	// MagicBuff is a pointer to an interface{}
	MagicBuff interface{}
}

type SnapshotdataLocationGet struct {
	DevID      DevID
	RangeCount uint32
	Ranges     []Range
}

type SnapshotdataLocationComplete struct {
	DevID DevID
}

// persistent CBT data parameter
type PersistentCBTData struct {
	Size uint32
	// Parameter is a pointer to a char*
	Parameter *byte
}
