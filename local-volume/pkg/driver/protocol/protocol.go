package protocol

// UnixSocket used to communicate between client and server
// Must match the driver pod host mount
// TODO: variabilize?
const UnixSocket = "/var/run/elastic-local/socket"

type MountRequest struct {
	TargetDir string            `json:"targetDir"`
	Options   map[string]string `json:"options,omitempty"`
}

type UnmountRequest struct {
	TargetDir string `json:"targetDir"`
}
