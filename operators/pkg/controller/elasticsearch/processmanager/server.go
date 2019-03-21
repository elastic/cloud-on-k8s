// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"context"
	"encoding/json"
	_ "expvar"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
)

const (
	shutdownTimeout = 5 * time.Second
	HTTPPort        = ":8080"
)

// ProcessServer is an HTTP server that can access to the managed process and the keystore updater.
type ProcessServer struct {
	*http.Server

	cfg       Config
	esProcess *Process
	ksUpdater *keystore.Updater
}

// NewServer creates a new ProcessServer.
func NewProcessServer(cfg Config, process *Process, updater *keystore.Updater) *ProcessServer {
	mux := http.NewServeMux()
	s := ProcessServer{
		&http.Server{
			Addr:    HTTPPort,
			Handler: mux,
		},
		cfg,
		process,
		updater,
	}

	mux.HandleFunc("/health", s.Health)
	mux.HandleFunc("/es/start", s.EsStart)
	mux.HandleFunc("/es/stop", s.EsStop)
	mux.HandleFunc("/es/status", s.EsStatus)
	mux.HandleFunc("/keystore/status", s.KeystoreStatus)

	return &s
}

// Start starts the HTTP server in the background.
// The current program exits if an error occurred.
func (s *ProcessServer) Start() {
	go func() {
		var err error
		if s.cfg.EnableTLS {
			err = s.ListenAndServeTLS(s.cfg.CertPath, s.cfg.KeyPath)
		} else {
			err = s.ListenAndServe()
		}

		if err != nil {
			if err == http.ErrServerClosed {
				log.Info("HTTP server closed")
			} else {
				log.Error(err, "Could not start HTTP server")
				os.Exit(1)
			}
		}
		return
	}()
}

// Stop stops the HTTP server.
func (s *ProcessServer) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		log.Error(err, "Fail to stop HTTP server")
	}
	log.Info("HTTP server stopped")
}

func (s *ProcessServer) Health(w http.ResponseWriter, req *http.Request) {
	ok(w, "ping")
}

func (s *ProcessServer) EsStart(w http.ResponseWriter, req *http.Request) {
	state, err := s.esProcess.Start()
	if err != nil {
		log.Info("Failed to start es process", "state", state, "err", err.Error())
		//ko(w, state.String())
		//return
		w.WriteHeader(http.StatusInternalServerError)
	}

	if state == starting {
		w.WriteHeader(http.StatusAccepted)
	}

	status, err := s.esProcess.Status()
	if err != nil {
		ko(w, "Failed to get es status while starting process: "+err.Error())
		return
	}

	jsonOk(w, status)
}

func (s *ProcessServer) EsStop(w http.ResponseWriter, req *http.Request) {
	var err error

	killHard := false
	forceParam := req.URL.Query().Get("force")
	if forceParam != "" {
		killHard, err = strconv.ParseBool(forceParam)
		if err != nil {
			log.Error(err, "Fail to stop")
			ko(w, "Invalid `force` query parameter")
			return
		}
	}

	killHardTimeout := 0
	timeoutParam := req.URL.Query().Get("timeout")
	if timeoutParam != "" {
		hardKillTimeoutSeconds, err := strconv.Atoi(timeoutParam)
		if err != nil {
			log.Error(err, "Fail to stop")
			ko(w, "Invalid `timeout` query parameter")
			return
		}
		if hardKillTimeoutSeconds < 0 {
			log.Error(err, "Fail to stop")
			ko(w, "Invalid `timeout` query parameter, must be greater than 0.")
			return
		}
		killHardTimeout = hardKillTimeoutSeconds
	}

	state, err := s.esProcess.Stop(killHard, time.Duration(killHardTimeout)*time.Second)
	if err != nil {
		log.Info("Failed to stop es process", "state", state, "err", err.Error())
		//ko(w, state.String())
		//return
		w.WriteHeader(http.StatusInternalServerError)
		//ko(w, state.String())
		return
	}

	if state == stopping || state == killing {
		w.WriteHeader(http.StatusAccepted)
	}

	status, err := s.esProcess.Status()
	if err != nil {
		ko(w, "Failed to get es status while stopping process: "+err.Error())
		return
	}

	jsonOk(w, status)
}

func (s *ProcessServer) EsStatus(w http.ResponseWriter, req *http.Request) {
	status, err := s.esProcess.Status()
	if err != nil {
		ko(w, "Failed to get es status: "+err.Error())
		return
	}

	jsonOk(w, status)
}

func (s *ProcessServer) KeystoreStatus(w http.ResponseWriter, req *http.Request) {
	status, err := s.ksUpdater.Status()
	if err != nil {
		ko(w, "Failed to get keystore updater status: "+err.Error())
		return
	}

	jsonOk(w, status)
}

// HTTP utilities

func ok(w http.ResponseWriter, msg string) {
	//log.Info("HTTP response", "status", "Ok", "msg", msg)
	write(w, http.StatusOK, msg)
}

func jsonOk(w http.ResponseWriter, obj interface{}) {
	//log.Info("HTTP response", "status", "Ok", "msg", msg)
	bytes, _ := json.Marshal(obj)
	_, _ = w.Write(bytes)
}

func ko(w http.ResponseWriter, msg string) {
	//log.Info("HTTP response", "status", "Error", "msg", msg)
	write(w, http.StatusInternalServerError, fmt.Sprintf(`{"error": "%s"}`, msg))
}

func write(w http.ResponseWriter, statusCode int, msg string) {
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(msg))
}
