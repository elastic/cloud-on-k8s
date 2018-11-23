package protocol

// UnixSocket used to communicate between client and server
// Must match the driver pod volume mount on the host
// TODO: variabilize?
const UnixSocket = "/var/run/elastic-local/socket"

// MountRequest is the request format to use for client-daemon communication
// when mounting a PersistentVolume
type MountRequest struct {
	TargetDir string            `json:"targetDir"`
	Options   map[string]string `json:"options,omitempty"`
}

// UnmountRequest is the request format to use for client-daemon communication
// when unmounting a PersistentVolume
type UnmountRequest struct {
	TargetDir string `json:"targetDir"`
}
