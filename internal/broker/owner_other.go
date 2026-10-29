//go:build !unix

package broker

import "io/fs"

// ownerUID cannot tell the owner of a file on this platform, so that every
// ownership check fails.
func ownerUID(fs.FileInfo) (int, bool) {
	return 0, false
}
