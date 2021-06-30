package system

import (
	"bufio"
	"os"
	"strings"

	"github.com/pkg/errors"
)

type OSDetails struct {
	Name    string
	Version string
}

func FetchOSDetails() (OSDetails, error) {
	fd, err := os.Open("/etc/os-release")
	if err != nil {
		return OSDetails{}, errors.Wrap(err, "opening /etc/os-release")
	}
	scanner := bufio.NewScanner(fd)

	ret := OSDetails{}
	for scanner.Scan() {
		line := scanner.Text()
		cols := strings.SplitN(line, "=", 2)
		if len(cols) != 2 {
			continue
		}

		switch cols[0] {
		case "ID":
			ret.Name = cols[1]
		case "VERSION_ID":
			ret.Version = strings.Trim(cols[1], "\"")
		}
	}
	return ret, nil
}
