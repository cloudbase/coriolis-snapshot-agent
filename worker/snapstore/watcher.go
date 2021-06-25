package snapstore

import (
	"encoding/binary"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	vErrors "coriolis-veeam-bridge/errors"
	"coriolis-veeam-bridge/internal/ioctl"
	"coriolis-veeam-bridge/internal/storage"
	"coriolis-veeam-bridge/internal/types"
	"coriolis-veeam-bridge/internal/util"
	"coriolis-veeam-bridge/worker/common"
)

// REMOVE. Instantiate the watcher struct from the wather manager.
// func NewSnapStoreWatcher(deviceID, snapDevice types.DevID, basedir string, errChan chan error) (*Watcher, error) {
// 	newID := uuid.New()
// 	charDev, err := os.OpenFile(ioctl.VEEAM_DEV, os.O_RDWR, 0600)
// 	if err != nil {
// 		return nil, errors.Wrapf(err, "opening char device %s", ioctl.VEEAM_DEV)
// 	}

// 	w := &Watcher{
// 		ID:           newID,
// 		devID:        deviceID,
// 		snapDeviceID: snapDevice,
// 		basedir:      basedir,

// 		charDevice: charDev,

// 		charDeviceReaderQuit: make(chan struct{}),
// 		errChan:              errChan,
// 	}
// 	go w.charDeviceReader()

// 	if err := w.create(); err != nil {
// 		// stop the reader and return the error.
// 		charDev.Close()
// 		return nil, errors.Wrap(err, "creating snap store")
// 	}

// 	if err := w.allocateInitialStorage(); err != nil {
// 		// stop the reader and return the error.
// 		w.cleanupSnapStore()
// 		defer charDev.Close()
// 		return nil, errors.Wrap(err, "creating snap store")
// 	}
// 	return w, nil
// }

func NewSnapStoreWatcher(param common.CreateSnapStoreParams, watcherChan chan interface{}) (*Watcher, error) {
	charDev, err := os.OpenFile(ioctl.VEEAM_DEV, os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrapf(err, "opening char device %s", ioctl.VEEAM_DEV)
	}
	asUUID := uuid.UUID(param.ID)
	watcher := &Watcher{
		ID:                   asUUID,
		snapStoreFileSize:    param.SnapStoreFileSize,
		devID:                param.DeviceID,
		snapDeviceID:         param.SnapDeviceID,
		basedir:              param.BaseDir,
		charDevice:           charDev,
		messageChan:          watcherChan,
		charDeviceReaderQuit: make(chan struct{}),
	}

	if err := watcher.Start(); err != nil {
		return nil, errors.Wrapf(err, "starting watcher for snapstore %s", asUUID.String())
	}
	return watcher, nil
}

type Watcher struct {
	ID                uuid.UUID
	snapStoreFileSize uint64
	devID             types.DevID
	snapDeviceID      types.DevID
	// basedir is the folder in which we can automatically
	// allocate new ranges.
	basedir string

	charDevice *os.File

	// Keep a local cache of how much disk space was allocated
	// to this snap store.
	allocatedSpace int64

	charDeviceReaderQuit chan struct{}
	messageChan          chan interface{}
}

func (w *Watcher) Start() error {
	if err := w.create(); err != nil {
		return errors.Wrapf(err, "creating snap store %s", w.ID.String())
	}

	go w.charDeviceReader()
	return nil
}

func (w *Watcher) removeSnapStoreFiles() error {
	log.Printf("removing basedir: %s", w.basedir)
	if err := os.RemoveAll(w.basedir); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return errors.Wrap(err, "removing files")
		}
	}

	w.messageChan <- common.SnapStoreDeletedMessage{
		SnapStoreID: w.ID,
	}
	return nil
}

func (w *Watcher) GetTrackedDeviceSize() (uint64, error) {
	dev, err := storage.FindBlockVolumeByID(w.devID.Major, w.devID.Minor)
	if err != nil {
		return 0, errors.Wrap(err, "fetching device")
	}
	return uint64(dev.Size), nil
}

func (w *Watcher) create() error {
	log.Printf("empty limit for device %d:%d is %d bytes", w.devID.Major, w.devID.Minor, w.snapStoreFileSize)
	snapStoreParams := SnapStoreStretchInitiateParams{
		ID:                [16]byte(w.ID),
		EmptyLimit:        w.snapStoreFileSize,
		SnapStoreDeviceID: w.snapDeviceID,
		Count:             1,
		DeviceIDs: []types.DevID{
			w.devID,
		},
	}
	asBytes := snapStoreParams.Serialize()

	wr, err := w.charDevice.Write(asBytes)
	if err != nil {
		return errors.Wrap(err, "writing to character device")
	}

	if wr != len(asBytes) {
		return errors.Errorf("failed to write to character device (have %d, wrote %d)", len(asBytes), wr)
	}

	return nil
}

func (w *Watcher) AllocateInitialStorage() (string, uint64, error) {
	deviceSize, err := w.GetTrackedDeviceSize()
	if err != nil {
		return "", 0, errors.Wrap(err, "fetching device size")
	}

	// allocate 20% of device size.
	toAllocate := uint64(float64(deviceSize) * 0.2)
	return w.AllocateStorage(toAllocate)
}

func (w *Watcher) AllocateStorage(size uint64) (string, uint64, error) {

	if _, err := os.Stat(w.basedir); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return "", 0, errors.Wrapf(err, "checking basedir %s", w.basedir)
		}
		if err := os.MkdirAll(w.basedir, 00770); err != nil {
			return "", 0, errors.Wrapf(err, "creating basedir %s", w.basedir)
		}
	}

	fsInfo, err := util.GetFileSystemInfoFromPath(w.basedir)
	if err != nil {
		return "", 0, errors.Wrap(err, "fetching FS info")
	}

	if fsInfo.BytesFree < size {
		return "", 0, vErrors.NewValueError("could not allocate %d bytes. Only %d bytes available on device", size, fsInfo.BytesFree)
	}

	newFileName := uuid.New()
	filePath := filepath.Join(w.basedir, newFileName.String())

	if err := util.CreateSnapStoreFile(filePath, size); err != nil {
		return "", 0, errors.Errorf("failed to create %s: %+v", filePath, err)
	}

	ranges, devID, err := util.GetFileRanges(filePath)
	if err != nil {
		if err != nil {
			return "", 0, errors.Wrap(err, "fetching file ranges")
		}
	}

	if devID != w.snapDeviceID {
		return "", 0, vErrors.NewInvalidDeviceErr("snap device %d:%d differs from snap file location device %d:%d", w.snapDeviceID.Major, w.snapDeviceID.Minor, devID.Major, devID.Minor)
	}

	params := NextPortionParams{
		ID:     w.ID,
		Count:  uint32(len(ranges)),
		Ranges: ranges,
	}

	msgBytes := params.Serialize()
	wr, err := w.charDevice.Write(msgBytes)
	if err != nil {
		return "", 0, errors.Wrap(err, "adding file to snap store")
	}

	if len(msgBytes) != wr {
		return "", 0, errors.Errorf("written bytes length (%d) does not match message bytes length (%d)", wr, len(msgBytes))
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return "", 0, errors.Wrap(err, "fetching file info")
	}

	w.allocatedSpace += info.Size()
	return filePath, uint64(info.Size()), nil
}

func (w *Watcher) charDeviceReader() {
	defer close(w.charDeviceReaderQuit)

	for {
		// Largest command (ctrl_pipe_request_overflow) is 16 bytes
		// in length. Double that, just in case.
		buff := make([]byte, 32)

		n, err := w.charDevice.Read(buff)
		if err != nil {
			if errors.Is(err, os.ErrClosed) {
				log.Print("character device was closed; terminating char device reader")
				return
			}
			log.Printf("error reading from char device: %+v", err)
			// return error back to the main function and decide what to do there.
			w.messageChan <- common.ErrorMessage{
				SnapstoreID: w.ID,
				Error:       err,
			}
			w.charDevice.Close()
			return
		}
		if n < 4 {
			log.Printf("got invalid buffer length from character device: %d", n)
			continue
		}
		command := binary.LittleEndian.Uint32(buff[:4])

		switch command {
		case CHARCMD_ACKNOWLEDGE:
			// Currently this type of message is only sent out
			// after the creation of a snap store through the
			// character device. The result of that operation is
			// made known to the caller of Write()/ioctl through
			// the return value of each respective function.
			// We check the error code here and quit the worker
			// in case of an error. The caller of the create
			// operation already has the status of the call, and
			// can cleanup.
			code := binary.LittleEndian.Uint32(buff[4:])
			log.Printf("got acknowledge message with exit code %d for snapstore %s (%d:%d)", code, w.ID.String(), w.devID.Major, w.devID.Minor)
			if code != 0 {
				log.Printf("exiting watcher for snap store %s", w.ID.String())
				w.charDevice.Close()
				return
			}
			continue
		case CHARCMD_INVALID:
			// This call is not used anywhere in the kernel module.
			log.Printf("got CHARCMD_INVALID message type")
			continue
		case CHARCMD_HALFFILL:
			if err := w.halfFillHandler(buff[4:]); err != nil {
				log.Printf("got error from halfFill handler: %q", err)
				w.messageChan <- common.ErrorMessage{
					SnapstoreID: w.ID,
					Error:       err,
				}
			}
		case CHARCMD_OVERFLOW:
			// it's likely that the snapshot image is now corrupt,
			// and the snapshot needs to be deleted.
			if err := w.overflowHandler(buff[4:]); err != nil {
				log.Printf("got error from overflow handler: %q", err)
				w.messageChan <- common.ErrorMessage{
					SnapstoreID: w.ID,
					Error:       err,
				}
			}
		case CHARCMD_TERMINATE:
			// Snap store we are watching has been deleted. We can quit.
			log.Printf("got terminate notification for snap store %s", w.ID)
			if err := w.removeSnapStoreFiles(); err != nil {
				log.Printf("failed to delete files for snap store %s: %+v", w.ID, err)
				w.messageChan <- common.ErrorMessage{
					SnapstoreID: w.ID,
					Error:       err,
				}
			}
			w.charDevice.Close()
			return
		default:
			log.Printf("Unknown command: %d", command)
		}
	}
}

func (w *Watcher) halfFillHandler(msg []byte) error {
	log.Printf("Got halffill notification from kernel module for snapstore %s", w.ID.String())
	// the amount of filled bytes is an uint64
	// split up in 2, 32 bit ranges.
	fillStatus1 := binary.LittleEndian.Uint32(msg[0:4])
	fillStatus2 := binary.LittleEndian.Uint32(msg[4:8])
	// We need to combine the 2 bit ranges to get the actual value.
	filledStatusVal := uint64(fillStatus2)<<32 | uint64(fillStatus1)
	log.Printf("snapstore %s fill status is %d MB", w.ID.String(), filledStatusVal/1024/1024)

	filePath, size, err := w.AllocateStorage(w.snapStoreFileSize)
	if err != nil {
		return errors.Wrap(err, "allocating file")
	}

	w.messageChan <- common.SnapStoreAddFileMessage{
		SnapStoreID: w.ID,
		FilePath:    filePath,
		FileSize:    size,
		FillStatus:  filledStatusVal,
	}
	return nil
}

func (w *Watcher) overflowHandler(msg []byte) error {
	errorCode := binary.LittleEndian.Uint32(msg[:4])
	// the amount of filled bytes is an uint64
	// split up in 2, 32 bit values. We need to combine them
	// to get the actual value.
	fillStatus1 := binary.LittleEndian.Uint32(msg[4:8])
	fillStatus2 := binary.LittleEndian.Uint32(msg[8:12])
	filledStatusVal := uint64(fillStatus2)<<32 | uint64(fillStatus1)

	log.Printf("snap store %s fill status is: %d bytes", w.ID.String(), filledStatusVal)
	// We send a snapstore overflow error back to the manager, which in turn will delete
	// the snapshot, and mark the error in the database.

	w.messageChan <- common.SnapStoreOverflowMessage{
		SnapStoreID: w.ID,
		// TODO: define a proper overflow error.
		Error: errors.Errorf("overflow error code %d", errorCode),
	}
	return vErrors.NewSnapStoreOverflowError(
		"snap store %s has overflown with error code %d", w.ID.String(), errorCode)
}
