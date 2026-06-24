// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build windows

package timestamp

import (
	"syscall"
	"time"
)

// setCreationTime sets the Windows creation time of path to t, leaving the
// access and write times to os.Chtimes. It opens the file for attribute writes
// and calls SetFileTime with only the creation-time slot populated.
//
// Uses only the standard syscall package — no run-time dependency.
func setCreationTime(path string, t time.Time) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := syscall.CreateFile(
		p,
		syscall.FILE_WRITE_ATTRIBUTES,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)

	ft := syscall.NsecToFiletime(t.UnixNano())
	// Only the creation time (first arg) is set; nil leaves access/write times.
	return syscall.SetFileTime(h, &ft, nil, nil)
}
