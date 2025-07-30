// Package xattr provides OS-specific wrappers for setting extended attributes
// to mark files as ignored by Dropbox.
package xattr

import (
	"runtime"

	"golang.org/x/sys/unix"
)

const (
	// macOS FileProvider attribute
	attrMacOS = "com.apple.fileprovider.ignore#P"
	// Linux Dropbox attribute
	attrLinux = "com.dropbox.ignored"
	// Attribute value
	attrValue = "1"
)

// SetIgnored sets the appropriate extended attribute to mark a file/directory
// as ignored by Dropbox.
func SetIgnored(path string) error {
	name := getAttrName()
	return unix.Setxattr(path, name, []byte(attrValue), 0)
}

// IsIgnored checks if a file/directory has the Dropbox ignore attribute set.
func IsIgnored(path string) (bool, error) {
	name := getAttrName()
	sz, err := unix.Getxattr(path, name, nil)
	if isNoAttrError(err) {
		return false, nil
	}
	return sz >= 0 && err == nil, err
}

// RemoveIgnored removes the Dropbox ignore attribute from a file/directory.
func RemoveIgnored(path string) error {
	name := getAttrName()
	err := unix.Removexattr(path, name)
	if isNoAttrError(err) {
		return nil
	}
	return err
}

// getAttrName returns the appropriate attribute name for the current OS.
func getAttrName() string {
	if runtime.GOOS == "darwin" {
		return attrMacOS
	}
	return attrLinux
}