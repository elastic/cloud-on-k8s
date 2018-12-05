package daemon

import (
	"encoding/json"
	"net/http"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/pathutil"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	log "github.com/sirupsen/logrus"
)

// SetupRoutes returns an http ServeMux to handle all our HTTP routes
func (s *Server) SetupRoutes() *http.ServeMux {
	handler := http.NewServeMux()
	handler.HandleFunc("/init", s.InitHandler())
	handler.HandleFunc("/mount", s.MountHandler())
	handler.HandleFunc("/unmount", s.UnmountHandler())
	return handler
}

// InitHandler handles init HTTP calls
func (s *Server) InitHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Init request")

		resp := s.driver.Init()
		log.Infof("%+v", resp)

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			err500(w, err)
		}
	}
}

// MountHandler handles mount HTTP calls
func (s *Server) MountHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Mount request")

		defer r.Body.Close()
		var params protocol.MountRequest
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			err500(w, err)
			return
		}

		// Start by updating PV with the current node name.
		// The reason why we do this before mounting the volume is error handling.
		// If we do it the other way around (mount first, then update PV), then what
		// should we do if we cannot update the PV (eg. APIServer cannot be reached,
		// or resource disappeared)?
		// Moreover, the pod was assigned to this node and this won't change.
		pvName := pathutil.ExtractPVCID(params.TargetDir)
		log.Infof("Updating PV %s with affinity for node %s", pvName, s.nodeName)
		if _, err := s.k8sClient.BindPVToNode(pvName, s.nodeName); err != nil {
			log.WithError(err).Error("Cannot update Persistent Volume node affinity")
			err500(w, err)
			return
		}

		// Then, mount the volume
		log.Info("Mounting volume to the host")
		resp := s.driver.Mount(params)
		log.Infof("%+v", resp)

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			err500(w, err)
			return
		}
	}
}

// UnmountHandler handles unmount HTTP calls
func (s *Server) UnmountHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Unmount request")

		defer r.Body.Close()
		var params protocol.UnmountRequest
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			err500(w, err)
			return
		}

		resp := s.driver.Unmount(params)
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
