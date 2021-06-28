package util

import (
	"fmt"
	"io"
	"os"
	"path"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	"coriolis-snapshot-agent/config"
)

// GetLoggingWriter returns a new io.Writer suitable for logging.
func GetLoggingWriter(cfg *config.Config) (io.Writer, error) {
	var writer io.Writer = os.Stdout
	if cfg.LogFile != "" {
		dirname := path.Dir(cfg.LogFile)
		if _, err := os.Stat(dirname); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to create log folder")
			}
			if err := os.MkdirAll(dirname, 0o711); err != nil {
				return nil, fmt.Errorf("failed to create log folder")
			}
		}
		writer = &lumberjack.Logger{
			Filename:   cfg.LogFile,
			MaxSize:    500, // megabytes
			MaxBackups: 3,
			MaxAge:     28,   //days
			Compress:   true, // disabled by default
		}
	}
	return writer, nil
}
