package storage

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	udev "github.com/farjump/go-libudev"
	"github.com/pkg/errors"
)

type DeviceStatus string

const (
	DeviceStatusActive   DeviceStatus = "active"
	DeviceStatusInactive DeviceStatus = "inactive"
	DeviceStatusUnknown  DeviceStatus = "unknown"
	udevActionAdd                     = "add"
	udevActionRemove                  = "remove"
)

type UdevDevice struct {
	DeviceNode   string
	DeviceStatus DeviceStatus
}

type UdevMonitor struct {
	cancel  context.CancelFunc
	ctx     context.Context
	monitor *udev.Monitor
	devices sync.Map
}

func NewUdevMonitor(ctx context.Context, cancel context.CancelFunc) (monitor *UdevMonitor) {
	u := udev.Udev{}
	m := u.NewMonitorFromNetlink("udev")
	m.FilterAddMatchSubsystemDevtype("block", "disk")
	ret := &UdevMonitor{
		cancel:  cancel,
		ctx:     ctx,
		monitor: m,
		devices: sync.Map{},
	}

	return ret
}

func (m *UdevMonitor) Start() {
	ch, _ := m.monitor.DeviceChan(m.ctx)
	for d := range ch {
		device := UdevDevice{
			DeviceNode:   d.Devnode(),
		}
		devKey := fmt.Sprintf("%d-%d", d.Devnum().Major(), d.Devnum().Minor())
		switch d.Action() {
		case udevActionAdd:
			device.DeviceStatus = DeviceStatusActive
		case udevActionRemove:
			m.devices.Delete(devKey)
		default:
			device.DeviceStatus = DeviceStatusUnknown
		}

		log.Printf("Udev device event detected, adding to monitor devices: %s -> %+v", devKey, device)
		m.devices.Store(devKey, device)
	}
}

func (m *UdevMonitor) Stop() {
	m.cancel()
}

func (m *UdevMonitor) GetUdevDevice(major int, minor int) (UdevDevice, error) {
	devKey := fmt.Sprintf("%d-%d", major, minor)
	attempts := 0
	for {
		if attempts >= 60 {
			return UdevDevice{}, errors.Errorf("Failed to detect device %d:%d", major, minor)
		}
		if foundDev, ok := m.devices.Load(devKey); ok {
			device := foundDev.(UdevDevice)
			log.Printf("Detected udev device with ID %d:%d -> %+v", major, minor, device)
			return device, nil
		}
		attempts++
		time.Sleep(1 * time.Second)
	}
}
