package daemon

import (
	"encoding/json"
	"io/ioutil"
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

		err := json.NewEncoder(w).Encode(resp)
		if err != nil {
			err500(w, err)
		}
	}
}

// MountHandler handles mount HTTP calls
func MountHandler(driver drivers.Driver) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Mount request")

		body, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			err500(w, err)
			return
		}
		var params protocol.MountRequest
		err = json.Unmarshal(body, &params)
		if err != nil {
			err500(w, err)
			return
		}

		resp := driver.Mount(params)
		log.Infof("%+v", resp)

		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			err500(w, err)
		}
	}
}

// UnmountHandler handles unmount HTTP calls
func UnmountHandler(driver drivers.Driver) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Unmount request")

		body, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			err500(w, err)
			return
		}
		var params protocol.UnmountRequest
		err = json.Unmarshal(body, &params)
		if err != nil {
			err500(w, err)
			return
		}

		resp := driver.Unmount(params)
		log.Infof("%+v", resp)

		err = json.NewEncoder(w).Encode(resp)
		if err != nil {
			err500(w, err)
		}
	}
}

// err500 logs an error and writes it in the http response
func err500(w http.ResponseWriter, err error) {
	log.WithError(err).Error()
	http.Error(w, err.Error(), 500)
}
