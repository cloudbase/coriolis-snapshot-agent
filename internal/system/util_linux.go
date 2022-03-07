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
	defer fd.Close()
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
