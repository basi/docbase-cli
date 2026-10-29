//go:build unix

package broker

import (
	"io/fs"
	"syscall"
)

// ownerUID returns the user that owns the file.
func ownerUID(info fs.FileInfo) (int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}

	return int(stat.Uid), true
}
