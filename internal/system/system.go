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

package system

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	"coriolis-snapshot-agent/apiserver/params"
	"coriolis-snapshot-agent/worker/manager"
)

const (
	EFI_SYS_PATH   = "/sys/firmware/efi"
	NET_CLASS_PATH = "/sys/class/net"
	TUN_IFACE      = 65534
)

type CPUInfo struct {
	PhysicalCores int            `json:"physical_cores"`
	LogicalCores  int            `json:"logical_cores"`
	CPUInfo       []cpu.InfoStat `json:"cpu_info"`
}

type IfaceType string

const (
	IfaceTypePhysical    IfaceType = "physical_interface"
	IfaceTypeVirtual     IfaceType = "virtual_interface"
	IfaceTypeBridge      IfaceType = "bridge_interface"
	IfaceTypeBond        IfaceType = "bond_interface"
	IfaceTypeTun         IfaceType = "tunnel_interface"
	IfaceTypeUnsupported IfaceType = "unsupported_interface"
)

type NetworkInterface struct {
	InterfaceType IfaceType           `json:"interface_type"`
	Slaves        []*NetworkInterface `json:"slaves,omitempty"`
	HWAddress     string              `json:"mac_address"`
	IPAddresses   []string            `json:"ip_addresses"`
	Name          string              `json:"nic_name"`
}

type OSInfo struct {
	Platform  string `json:"platform"`
	OSName    string `json:"os_name"`
	OSVersion string `json:"os_version"`
}

type SystemInfo struct {
	Memory          mem.VirtualMemoryStat `json:"memory"`
	CPUs            CPUInfo               `json:"cpus"`
	NICs            []NetworkInterface    `json:"network_interfaces"`
	Disks           []params.BlockVolume  `json:"disks"`
	OperatingSystem OSInfo                `json:"os_info"`
	Hostname        string                `json:"hostname"`
	FirmwareType    string                `json:"firmware_type"`
}

func getOSInfo() (OSInfo, error) {
	var name string
	var version string
	osDetails, err := FetchOSDetails()
	if err != nil {
		name = ""
		version = ""
		log.Printf("failed to get os info: %+v", err)
	} else {
		name = osDetails.Name
		version = osDetails.Version
	}
	return OSInfo{
		Platform:  runtime.GOOS,
		OSName:    name,
		OSVersion: version,
	}, nil
}

func getCPUInfo() (CPUInfo, error) {
	info, err := cpu.Info()
	if err != nil {
		return CPUInfo{}, errors.Wrap(err, "fetching CPU info")
	}

	physCount, err := cpu.Counts(false)
	if err != nil {
		return CPUInfo{}, errors.Wrap(err, "fetching physical CPU count")
	}

	logicalCount, err := cpu.Counts(true)
	if err != nil {
		return CPUInfo{}, errors.Wrap(err, "fetching logical CPU count")
	}

	return CPUInfo{
		CPUInfo:       info,
		PhysicalCores: physCount,
		LogicalCores:  logicalCount,
	}, nil
}

func getInterfaceTypeCode(interfacePath string) (int, error) {
	var code int
	typePath := path.Join(interfacePath, "type")
	file, err := os.Open(typePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open path: %s", typePath)
	}
	defer file.Close()

	_, err = fmt.Fscanf(file, "%d", &code)
	if err != nil {
		return 0, fmt.Errorf("no interface type code found for interface: %s", interfacePath)
	}

	return code, nil
}

func getBridgeInterfaceSlaves(bridge NetworkInterface, nicNameMapping map[string]*NetworkInterface) ([]*NetworkInterface, error) {
	var slaves []*NetworkInterface

	interfacePath := path.Join(NET_CLASS_PATH, bridge.Name)
	interfaceLinkPath, err := filepath.EvalSymlinks(interfacePath)
	if err != nil {
		return []*NetworkInterface{}, errors.Wrap(err, "fetching network interface's symlink")
	}
	bridgedInterfacesPath := path.Join(interfaceLinkPath, "brif")
	pathInfo, err := os.Stat(bridgedInterfacesPath)
	if err != nil {
		return []*NetworkInterface{}, errors.Wrap(err, "fetching bridge slaves")
	}
	if pathInfo.IsDir() {
		files, err := ioutil.ReadDir(bridgedInterfacesPath)
		if err != nil {
			log.Printf("Could not list directory %v: %+v", bridgedInterfacesPath, err)
			return []*NetworkInterface{}, errors.Wrap(err, "listing bridge slaves directory")
		}

		for _, f := range files {
			slaves = append(slaves, nicNameMapping[f.Name()])
		}
		log.Printf("Interfaces linked to bridge %v: %+v\n", bridge.Name, slaves)
	} else {
		log.Println(bridgedInterfacesPath, "is not a directory. Skipping")
		return []*NetworkInterface{}, nil
	}

	return slaves, nil
}

func getNICInfo() ([]NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, errors.Wrap(err, "fetching interfaces")
	}

	nicNameMapping := make(map[string]*NetworkInterface)
	ret := []NetworkInterface{}
	for _, val := range ifaces {
		if val.Flags&net.FlagLoopback != 0 {
			log.Println("Skipping loopback network interface:", val.Name)
			continue
		}

		nic := NetworkInterface{
			InterfaceType: IfaceTypePhysical,
			HWAddress:     val.HardwareAddr.String(),
			Name:          val.Name,
		}

		interfacePath := path.Join(NET_CLASS_PATH, val.Name)
		interfaceLinkPath, err := filepath.EvalSymlinks(interfacePath)
		if err != nil {
			log.Printf("Could not get network interface's symlink: %+v", err)
			continue
		}

		if code, err := getInterfaceTypeCode(interfaceLinkPath); err == nil && code == TUN_IFACE {
			nic.InterfaceType = IfaceTypeTun
		} else if _, err = os.Stat(path.Join(interfaceLinkPath, "device")); err == nil {
			nic.InterfaceType = IfaceTypePhysical
		} else if _, err = os.Stat(path.Join(interfaceLinkPath, "bridge")); err == nil {
			nic.InterfaceType = IfaceTypeBridge
		} else if _, err = os.Stat(path.Join(interfaceLinkPath, "bonding")); err == nil {
			nic.InterfaceType = IfaceTypeBond
		} else if _, err = os.Stat(path.Join("/sys/devices/virtual/net", val.Name)); err == nil {
			nic.InterfaceType = IfaceTypeVirtual
		} else {
			nic.InterfaceType = IfaceTypeUnsupported
		}

		addrs, err := val.Addrs()
		if err != nil {
			return nil, errors.Wrap(err, "fetching IP addresses")
		}

		ifaceAddrs := []string{}
		for _, addr := range addrs {
			ifaceAddrs = append(ifaceAddrs, addr.String())
		}
		nic.IPAddresses = ifaceAddrs

		nicNameMapping[nic.Name] = &nic
		ret = append(ret, nic)
	}

	for idx, nic := range ret {
		if nic.InterfaceType == IfaceTypeBridge {
			slaves, err := getBridgeInterfaceSlaves(nic, nicNameMapping)
			if err != nil {
				continue
			}
			ret[idx].Slaves = slaves
		}
	}

	return ret, nil
}

func GetSystemInfo(mgr *manager.Snapshot) (SystemInfo, error) {
	cpuInfo, err := getCPUInfo()
	if err != nil {
		return SystemInfo{}, errors.Wrap(err, "fetching CPU info")
	}

	disks, err := mgr.ListDisks(false, false)
	if err != nil {
		return SystemInfo{}, errors.Wrap(err, "fetching disk info")
	}

	meminfo, err := mem.VirtualMemory()
	if err != nil {
		return SystemInfo{}, errors.Wrap(err, "fetching memory info")
	}

	nics, err := getNICInfo()
	if err != nil {
		return SystemInfo{}, errors.Wrap(err, "fetching nic info")
	}

	osInfo, err := getOSInfo()
	if err != nil {
		return SystemInfo{}, errors.Wrap(err, "fetching os info")
	}
	hostname, err := os.Hostname()
	if err != nil {
		return SystemInfo{}, errors.Wrap(err, "fetching hostname")
	}

	firmwareType := "bios"
	if _, err := os.Stat(EFI_SYS_PATH); err == nil {
		firmwareType = "efi"
	}
	return SystemInfo{
		CPUs:            cpuInfo,
		Disks:           disks,
		Memory:          *meminfo,
		NICs:            nics,
		OperatingSystem: osInfo,
		Hostname:        hostname,
		FirmwareType:    firmwareType,
	}, nil
}
