package system

import (
	"coriolis-snapshot-agent/internal/storage"
	"log"
	"net"
	"os"
	"runtime"

	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

type CPUInfo struct {
	PhysicalCores int            `json:"physical_cores"`
	LogicalCores  int            `json:"logical_cores"`
	CPUInfo       []cpu.InfoStat `json:"cpu_info"`
}

type NIC struct {
	HWAddress   string   `json:"mac_address"`
	IPAddresses []string `json:"ip_addresses"`
	Name        string   `json:"nic_name"`
}

type OSInfo struct {
	Platform  string `json:"platform"`
	OSName    string `json:"os_name"`
	OSVersion string `json:"os_version"`
}

type SystemInfo struct {
	Memory          mem.VirtualMemoryStat `json:"memory"`
	CPUs            CPUInfo               `json:"cpus"`
	NICs            []NIC                 `json:"network_interfaces"`
	Disks           []storage.BlockVolume `json:"disks"`
	OperatingSystem OSInfo                `json:"os_info"`
	Hostname        string                `json:"hostname"`
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

func getNICInfo() ([]NIC, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, errors.Wrap(err, "fetching interfaces")
	}

	ret := []NIC{}
	for _, val := range ifaces {
		if val.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := val.Addrs()
		if err != nil {
			return nil, errors.Wrap(err, "fetching IP addresses")
		}

		ifaceAddrs := []string{}
		for _, addr := range addrs {
			ifaceAddrs = append(ifaceAddrs, addr.String())
		}

		nic := NIC{
			HWAddress:   val.HardwareAddr.String(),
			Name:        val.Name,
			IPAddresses: ifaceAddrs,
		}

		ret = append(ret, nic)
	}
	return ret, nil
}

func GetSystemInfo() (SystemInfo, error) {
	cpuInfo, err := getCPUInfo()
	if err != nil {
		return SystemInfo{}, errors.Wrap(err, "fetching CPU info")
	}

	disks, err := storage.BlockDeviceList(false, false)
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
	return SystemInfo{
		CPUs:            cpuInfo,
		Disks:           disks,
		Memory:          *meminfo,
		NICs:            nics,
		OperatingSystem: osInfo,
		Hostname:        hostname,
	}, nil
}
