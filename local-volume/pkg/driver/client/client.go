package client

import (
	"encoding/json"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/model"
)

func Init() string {
	output, err := Get("/init")
	if err != nil {
		return err.Error()
	}
	return output
}

func Mount(args []string) string {
	// parse args
	directory := args[0]
	var options map[string]string
	if len(args) > 1 {
		if err := json.Unmarshal([]byte(args[1]), &options); err != nil {
			return err.Error()
		}
	}
	// make request
	reqBody := &model.MountRequest{
		TargetDir: directory,
		Options:   options,
	}
	output, err := Post("/mount", reqBody)
	if err != nil {
		return err.Error()
	}
	return string(output)
}

func Unmount(args []string) string {
	// parse args
	directory := args[0]
	// make request
	reqBody := &model.UnmountRequest{
		TargetDir: directory,
	}
	output, err := Post("/unmount", reqBody)
	if err != nil {
		return err.Error()
	}
	return string(output)
}
