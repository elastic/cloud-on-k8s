package protocol

import "fmt"

// UnixSocket used to communicate between client and server
// Must match the driver pod volume mount on the host
// TODO: variabilize?
const UnixSocket = "/var/run/elastic-local/socket"

// MountRequest is the request format to use for client-daemon communication
// when mounting a PersistentVolume
type MountRequest struct {
	TargetDir string       `json:"targetDir"`
	Options   MountOptions `json:"options,omitempty"`
}

// MountOptions are the options passed to the mount request,
// from the provisioner to the driver (through the kubelet)
type MountOptions struct {
	SizeBytes int64 `json:"sizeBytes,string"`
}

// AsStrMap converts the MountOptions to a map[str]str
func (m MountOptions) AsStrMap() map[string]string {
	return map[string]string{
		"sizeBytes": fmt.Sprintf("%d", m.SizeBytes),
	}
}

// UnmountRequest is the request format to use for client-daemon communication
// when unmounting a PersistentVolume
type UnmountRequest struct {
	TargetDir string `json:"targetDir"`
}
