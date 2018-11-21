package daemon

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/model"
	log "github.com/sirupsen/logrus"
)

// SetupRoutes returns an http ServeMux to handle all our HTTP routes
func SetupRoutes(driver Driver) *http.ServeMux {
	handler := http.NewServeMux()
	handler.HandleFunc("/init", InitHandler(driver))
	handler.HandleFunc("/mount", MountHandler(driver))
	handler.HandleFunc("/unmount", UnmountHandler(driver))
	return handler
}

// InitHandler handles init HTTP calls
func InitHandler(driver Driver) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Driver init")

		resp := driver.Init()

		output, _ := json.Marshal(resp)
		w.Write(output)
	}
}

// MountHandler handles mount HTTP calls
func MountHandler(driver Driver) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Driver mount")

		body, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			err500(w, err)
			return
		}
		var params model.MountRequest
		err = json.Unmarshal(body, &params)
		if err != nil {
			err500(w, err)
			return
		}

		resp := driver.Mount(params)

		output, _ := json.Marshal(resp)
		w.Write(output)
	}
}

// UnmountHandler handles unmount HTTP calls
func UnmountHandler(driver Driver) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Driver unmount")

		body, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			err500(w, err)
			return
		}
		var params model.UnmountRequest
		err = json.Unmarshal(body, &params)
		if err != nil {
			err500(w, err)
			return
		}

		resp := driver.Unmount(params)

		jsonResp, _ := json.Marshal(resp)
		w.Write(jsonResp)
	}
}

// err500 logs an error and writes it in the http response
func err500(w http.ResponseWriter, err error) {
	log.WithError(err).Error()
	http.Error(w, err.Error(), 500)
}
