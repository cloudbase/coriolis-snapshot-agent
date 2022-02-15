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

//go:build linux
// +build linux

package storage

/*
#cgo linux LDFLAGS: -lblkid -luuid
#include <blkid/blkid.h>
*/
import "C"

import (
	"fmt"

	veeamErrors "coriolis-snapshot-agent/errors"
)

// BlkIDProbe runs a probe on devName and returns a map which contains a map containing
// information about the volume.
func BlkIDProbe(devName string) (map[string]string, error) {
	ret := map[string]string{}
	blkidProbe := C.blkid_new_probe_from_filename(C.CString(devName))
	if blkidProbe == nil {
		return ret, fmt.Errorf("failed to create new blkid probe")
	}
	defer C.blkid_free_probe(blkidProbe)

	if err := C.blkid_probe_enable_partitions(blkidProbe, C.int(1)); err != 0 {
		return ret, fmt.Errorf("failed to enable partitions")
	}
	if err := C.blkid_probe_enable_superblocks(blkidProbe, C.int(1)); err != 0 {
		return ret, fmt.Errorf("failed to enable superblocks")
	}

	if err := C.blkid_probe_set_partitions_flags(blkidProbe, C.int(C.BLKID_PARTS_ENTRY_DETAILS)); err != 0 {
		return ret, fmt.Errorf("failed to enable BLKID_PARTS_ENTRY_DETAILS")
	}

	if err := C.blkid_do_fullprobe(blkidProbe); err != 0 {
		if err == 1 {
			return ret, veeamErrors.ErrNoInfo
		}
		return ret, fmt.Errorf("failed to probe: %v", err)
	}

	nvals, err := C.blkid_probe_numof_values(blkidProbe)
	if err != nil {
		return ret, fmt.Errorf("failed to get number of values: %v", err)
	}

	for i := 0; i < int(nvals); i++ {
		var name *C.char
		var data *C.char
		var length C.size_t
		if err := C.blkid_probe_get_value(blkidProbe, C.int(i), &name, &data, &length); err != 0 {
			continue
		}
		nName := C.GoString(name)
		nData := C.GoString(data)
		ret[nName] = nData
	}

	return ret, nil
}
