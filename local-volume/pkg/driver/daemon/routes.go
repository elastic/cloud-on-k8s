package daemon

import (
	"encoding/json"
	"net/http"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/drivers"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	log "github.com/sirupsen/logrus"
)

// SetupRoutes returns an http ServeMux to handle all our HTTP routes
func SetupRoutes(driver drivers.Driver) *http.ServeMux {
	handler := http.NewServeMux()
	handler.HandleFunc("/init", InitHandler(driver))
	handler.HandleFunc("/mount", MountHandler(driver))
	handler.HandleFunc("/unmount", UnmountHandler(driver))
	return handler
}

// InitHandler handles init HTTP calls
func InitHandler(driver drivers.Driver) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Init request")

		resp := driver.Init()
		log.Infof("%+v", resp)

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			err500(w, err)
		}
	}
}

// MountHandler handles mount HTTP calls
func MountHandler(driver drivers.Driver) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Mount request")

		defer r.Body.Close()
		var params protocol.MountRequest
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			err500(w, err)
			return
		}

		resp := driver.Mount(params)
		log.Infof("%+v", resp)

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			err500(w, err)
		}
	}
}

// UnmountHandler handles unmount HTTP calls
func UnmountHandler(driver drivers.Driver) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Unmount request")

		defer r.Body.Close()
		var params protocol.UnmountRequest
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			err500(w, err)
			return
		}

		resp := driver.Unmount(params)
		log.Infof("%+v", resp)

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			err500(w, err)
		}
	}
}

// err500 logs an error and writes it in the http response
func err500(w http.ResponseWriter, err error) {
	log.WithError(err).Error()
	http.Error(w, err.Error(), 500)
}
