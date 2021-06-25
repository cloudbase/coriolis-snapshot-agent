package common

import "github.com/google/uuid"

type SnapStoreOverflowMessage struct {
	SnapStoreID uuid.UUID

	Error error
}

type SnapStoreDeletedMessage struct {
	SnapStoreID uuid.UUID
}

type SnapStoreAddFileMessage struct {
	SnapStoreID uuid.UUID

	FilePath string
	FileSize uint64

	FillStatus uint64
}

type ErrorMessage struct {
	SnapstoreID uuid.UUID

	Error error
}
