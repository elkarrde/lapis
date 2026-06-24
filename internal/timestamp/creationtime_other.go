// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows

package timestamp

import "time"

// setCreationTime is a no-op on platforms without a userspace-settable creation
// time. On Linux the change time (ctime) is maintained by the kernel and cannot
// be set; macOS birth time is left untouched. The modification and access times
// are still set by os.Chtimes in Apply.
func setCreationTime(path string, t time.Time) error {
	return nil
}
