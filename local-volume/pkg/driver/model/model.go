package model

// Name of our persistent volume implementation
const Name = "volumes.k8s.elastic.co/elastic-local"

const (
	// VolumesPath is the path into which volumes should be created
	// Must match the driver pod host mount
	// TODO: parametrize?
	VolumesPath = "/mnt/elastic-local-volumes"
	// TODO: gke needs this
	// VolumesPath = "/mnt/disks"

	// UnixSocket used to communicate between client and server
	// Must match the driver pod host mount
	// TODO: parametrize?
	UnixSocket = "/var/run/elastic-local/socket"
)

// -- Flex volume protocol

type Status string

const (
	StatusSuccess      Status = "Success"
	StatusFailure      Status = "Failure"
	StatusNotSupported Status = "Not Supported"
)

type Response struct {
	Status  Status `json:"status"`
	Message string `json:"message"`
	Device  string `json:"device,omitempty"`

	Capabilities Capabilities `json:"capabilities,omitempty"`
}

type Capabilities struct {
	Attach bool `json:"attach"`
}

// -- Client-daemon protocol

type MountRequest struct {
	TargetDir string            `json:"targetDir"`
	Options   map[string]string `json:"options,omitempty"`
}

type UnmountRequest struct {
	TargetDir string `json:"targetDir"`
}
