package lvm

import (
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
)

// Init initializes the driver and returns a response
func (d *Driver) Init() flex.Response {
	return flex.Response{
		Status:  flex.StatusSuccess,
		Message: "driver is available",
		Capabilities: flex.Capabilities{
			Attach: false, // only implement mount and unmount
		},
	}
}
