package lvm

import (
	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
)

func (d *Driver) Init() flex.Response {
	return flex.Response{
		Status:  flex.StatusSuccess,
		Message: "driver is available",
		Capabilities: flex.Capabilities{
			Attach: false, // only implement mount and unmount
		},
	}
}
