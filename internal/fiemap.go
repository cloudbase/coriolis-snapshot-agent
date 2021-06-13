// Coriolis OVM exporter
// Copyright (C) 2021 Cloudbase Solutions SRL
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package internal

import (
	"os"

	"github.com/pkg/errors"
	fibmap "github.com/rancher/go-fibmap"
)

const (
	// FeCount is the default extent count we request
	FeCount = 8000
)

func walkFiemap(fd fibmap.FibmapFile) ([]fibmap.Extent, error) {
	var ret []fibmap.Extent
	for {
		var last uint64 = 0
		if len(ret) > 0 {
			last = ret[len(ret)-1].Logical + ret[len(ret)-1].Length
		}
		extents, err := fibmap.Fiemap(fd.Fd(), last, fibmap.FIEMAP_MAX_OFFSET, FeCount)
		if int(err) != 0 {
			return nil, errors.Wrap(err, "fetching fiemap")
		}

		if len(extents) == 0 {
			break
		}
		ret = append(ret, extents...)
	}
	return ret, nil
}

// GetExtents returns a list of extents allocated to the file
func GetExtents(filename string) ([]fibmap.Extent, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, errors.Wrap(err, "opening file")
	}
	defer fd.Close()
	fmFile := fibmap.NewFibmapFile(fd)

	return walkFiemap(fmFile)
}
