package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	vErrors "coriolis-snapshot-agent/errors"
	"coriolis-snapshot-agent/internal/storage"
	"coriolis-snapshot-agent/internal/types"
	"coriolis-snapshot-agent/internal/util"
)

const (
	// DefaultConfigFile is the default path to the OVM exporter config
	DefaultConfigFile = "/etc/coriolis-snapshot-agent/config.toml"

	// DefaultDBFile is the default location for the DB file.
	// We cannot persist CBT info and snapshots across reboots. Saving
	// the application state in an ephemeral folder saves us the trouble
	// of detecting a reboot and cleaning up stale data. We just recreate
	// the database from scratch and initialize snap stores, tracking, etc.
	DefaultDBFile = "/var/run/coriolis-snapshot-agent/coriolis-snapshot-agent.db"

	// DefaultListenPort is the default HTTPS listen port
	DefaultListenPort = 8899

	// DefaultSnapStoreFileSize is the default allocation size for new chunks that get
	// added to a snap store.
	DefaultSnapStoreFileSize uint64 = 2 * 1024 * 1024 * 1024 // 2GB
)

// ParseConfig parses the file passed in as cfgFile and returns
// a *Config object.
func ParseConfig(cfgFile string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(cfgFile, &config); err != nil {
		return nil, errors.Wrap(err, "decoding toml")
	}

	if config.CoWDestination == nil || len(config.CoWDestination) == 0 {
		return nil, fmt.Errorf("cow_destination is mandatory")
	}

	var devices []types.DevID
	for _, val := range config.CoWDestination {
		devInfo, err := util.GetBlockDeviceInfoFromFile(val)
		if err != nil {
			return nil, errors.Wrap(err, "fetching cow destination info")
		}

		devices = append(devices, types.DevID{
			Major: devInfo.Major,
			Minor: devInfo.Minor,
		})
	}

	devPaths, err := util.FindAllInvolvedDevices(devices)
	if err != nil {
		return nil, errors.Wrap(err, "determining device paths")
	}
	config.cowDestinationDevicePaths = devPaths

	if config.DBFile == "" {
		config.DBFile = DefaultDBFile
	}

	if config.SnapStoreFileSize == 0 {
		config.SnapStoreFileSize = DefaultSnapStoreFileSize
	}

	if err := config.Validate(); err != nil {
		return nil, errors.Wrap(err, "validating config")
	}
	return &config, nil
}

type SnapStoreMapping struct {
	Device   string `toml:"device"`
	Location string `toml:"location"`
}

func (s *SnapStoreMapping) Validate() error {
	if s.Device == "" || s.Location == "" {
		return vErrors.NewValueError("invalid device or location in mapping")
	}

	if _, err := os.Stat(s.Location); err != nil {
		return errors.Wrapf(err, "location %s does not exist", s.Location)
	}

	if _, err := os.Stat(filepath.Join("/dev", s.Device)); err != nil {
		return vErrors.NewValueError("invalid device %s in mapping", s.Device)
	}
	return nil
}

// Config is the coriolis-snapshot-agent config
type Config struct {
	// DBFile is the path on disk to the database location
	DBFile string `toml:"db_file"`
	// APIServer is the api server configuration.
	APIServer APIServer `toml:"api"`
	// LogFile is the location of the log file
	LogFile string `toml:"log_file"`
	// CoWDestination is the path to a folder where snap storage
	// extents will be pre-allocated via files. This folder must
	// live on a separate disk, which will be excluded from being
	// snapshotted.
	//
	// Note: If the destination is on a device mapper, all disks that
	// compose that device mapper will also be excluded.
	//
	// In future versions, you will be able to host these folders
	// on disks that do take part of the snapshotting process.
	CoWDestination []string `toml:"snapstore_destinations"`
	// AutoInitPhysicalDisks if set tot true, will add all physical
	// disks to tracking when service starts. Device mappers will be
	// skipped, as well as any virtual devices (loop, ram, etc).
	AutoInitPhysicalDisks bool `toml:"auto_init_physical_disks"`
	// SnapStoreMappings is a pre-configured list of device to snapstore
	// mappings.
	SnapStoreMappings []SnapStoreMapping `toml:"snapstore_mapping"`
	SnapStoreFileSize uint64             `toml:"snap_store_file_size"`

	cowDestinationDevicePaths []string
}

func (c *Config) CowDestinationDevices() []string {
	return c.cowDestinationDevicePaths
}

// Validate validates the config options
func (c *Config) Validate() error {
	if c.DBFile == "" {
		return fmt.Errorf("missing db_file")
	}

	parentDir := filepath.Dir(c.DBFile)
	if _, err := os.Stat(parentDir); err != nil {
		return errors.Wrapf(err, "db file parent dir %s does not exist", parentDir)
	}

	parentDirInfo, err := util.GetFileSystemInfoFromPath(parentDir)
	if err != nil {
		return errors.Wrap(err, "getting DB dir info")
	}

	if parentDirInfo.Type != storage.TMPFS_MAGIC {
		return vErrors.NewValueError("database file path is not on a tmpfs filesystem")
	}

	if err := c.APIServer.Validate(); err != nil {
		return errors.Wrap(err, "validating api server section")
	}

	for _, mapping := range c.SnapStoreMappings {
		found := false
		for _, location := range c.CoWDestination {
			if location == mapping.Location {
				found = true
				break
			}
		}
		if !found {
			return vErrors.NewValueError("mapping %s points to invalid cow destination", mapping.Location)
		}
	}

	return nil
}

// APIServer holds configuration for the API server
// worker
type APIServer struct {
	Bind      string    `toml:"bind"`
	Port      int       `toml:"port"`
	TLSConfig TLSConfig `toml:"tls"`
}

// BindAddress returns a host:port string.
func (a *APIServer) BindAddress() string {
	return fmt.Sprintf("%s:%d", a.Bind, a.Port)
}

// Validate validates the API server config
func (a *APIServer) Validate() error {
	if a.Port > 65535 || a.Port < 1 {
		return fmt.Errorf("invalid port nr %q", a.Port)
	}

	ip := net.ParseIP(a.Bind)
	if ip == nil {
		// No need for deeper validation here, as any invalid
		// IP address specified in this setting will raise an error
		// when we try to bind to it.
		return fmt.Errorf("invalid IP address")
	}
	if err := a.TLSConfig.Validate(); err != nil {
		return errors.Wrap(err, "validating TLS config")
	}
	return nil
}

// TLSConfig is the API server TLS config
type TLSConfig struct {
	Cert   string `toml:"certificate"`
	Key    string `toml:"key"`
	CACert string `toml:"ca_certificate"`
}

// Validate validates the TLS config
func (t *TLSConfig) Validate() error {
	if _, err := t.TLSConfig(); err != nil {
		return err
	}
	return nil
}

// TLSConfig returns a *tls.Config for the ovm exporter server
func (t *TLSConfig) TLSConfig() (*tls.Config, error) {
	caCertPEM, err := ioutil.ReadFile(t.CACert)
	if err != nil {
		return nil, err
	}

	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM(caCertPEM)
	if !ok {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	cert, err := tls.LoadX509KeyPair(t.Cert, t.Key)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    roots,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, nil
}

// Dump dumps the config to a file
func (c *Config) Dump(destination string) error {
	fd, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE, 00700)
	if err != nil {
		return err
	}

	enc := toml.NewEncoder(fd)
	if err := enc.Encode(c); err != nil {
		return err
	}
	return nil
}
