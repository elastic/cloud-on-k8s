package lvm

import "strings"

// simpleError wraps a string into an error
type simpleError string

func (s simpleError) Error() string { return string(s) }

// Known LVM Errors
const (
	ErrNoSpace                = simpleError("lvm: not enough free space")
	ErrTooFewDisks            = simpleError("lvm: not enough underlying devices")
	ErrPhysicalVolumeNotFound = simpleError("lvm: physical volume not found")
	ErrVolumeGroupNotFound    = simpleError("lvm: volume group not found")
	ErrLogicalVolumeNotFound  = simpleError("lvm: logical volume not found")
	ErrInvalidLVName          = simpleError("lvm: name contains invalid character, valid set includes: [A-Za-z0-9_+.-]")
)

// isVolumeGroupNotFound returns true if the error is due to the vg not being found
func isVolumeGroupNotFound(err error) bool {
	const prefix = "Volume group"
	const suffix = "not found"
	lines := strings.Split(err.Error(), "\n")
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(line, suffix) {
			return true
		}
	}
	return false
}

// isInsufficientSpace returns true if the error is due to insufficient space
func isInsufficientSpace(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "insufficient free space")
}

// isInsufficientDevices returns true if the error is due to insufficient underlying devices
func isInsufficientDevices(err error) bool {
	return strings.Contains(err.Error(), "Insufficient suitable allocatable extents for logical volume")
}

// isVolumeGroupNotFound returns true if the error is due to the lv not being found
func isLogicalVolumeNotFound(err error) bool {
	const prefix = "Failed to find logical volume"
	lines := strings.Split(err.Error(), "\n")
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
