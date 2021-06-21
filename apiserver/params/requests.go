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
