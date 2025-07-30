// +build linux

package xattr

import "golang.org/x/sys/unix"

// isNoAttrError checks if the error indicates the attribute doesn't exist
func isNoAttrError(err error) bool {
	return err == unix.ENODATA
}