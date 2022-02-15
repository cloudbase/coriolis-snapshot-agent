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

package params

type AddTrackedDiskRequest struct {
	DevicePath string `json:"device_path"`
}

type CreateSnapStoreMappingRequest struct {
	SnapStoreLocation string `json:"snapstore_location_id"`
	TrackedDisk       string `json:"tracked_disk_id"`
}

type AddSnapStoreStorageRequest struct {
	SnapStoreID string `json:"snapstore_id"`
	Size        int64  `json:"size_bytes"`
}

type CreateSnapshotRequest struct {
	TrackedDiskIDs []string `json:"tracked_disk_ids"`
}
