package client

import (
	"encoding/json"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
)

// Init performs a call to the /init path using the client.
func Init(c Caller) string {
	output, err := c.Get("/init")
	if err != nil {
		return err.Error()
	}
	return output
}

// Mount performs a call to the /mount path using the client.
func Mount(c Caller, args []string) string {
	// parse args
	directory := args[0]
	var options protocol.MountOptions
	if len(args) > 1 {
		if err := json.Unmarshal([]byte(args[1]), &options); err != nil {
			return err.Error()
		}
	}
	// make request
	reqBody := &protocol.MountRequest{
		TargetDir: directory,
		Options:   options,
	}
	output, err := c.Post("/mount", reqBody)
	if err != nil {
		return err.Error()
	}
	return string(output)
}

// Unmount performs a call to the /unmount path using the client.
func Unmount(c Caller, args []string) string {
	// parse args
	directory := args[0]
	// make request
	reqBody := &protocol.UnmountRequest{
		TargetDir: directory,
	}
	output, err := c.Post("/unmount", reqBody)
	if err != nil {
		return err.Error()
	}
	return string(output)
}
