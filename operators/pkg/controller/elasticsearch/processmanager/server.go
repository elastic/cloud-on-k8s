// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
)

const (
	shutdownTimeout = 5 * time.Second
)

// ProcessServer is an HTTP server that can access to the managed process and the keystore updater.
type ProcessServer struct {
	*http.Server

	cfg       *Config
	esProcess *Process
	ksUpdater *keystore.Updater
}

// NewProcessServer creates a new ProcessServer.
func NewProcessServer(cfg *Config, process *Process, updater *keystore.Updater) *ProcessServer {
	mux := http.NewServeMux()
	s := ProcessServer{
		&http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
			Handler: mux,
		},
		cfg,
		process,
		updater,
	}

	mux.HandleFunc("/health", s.Health)
	mux.HandleFunc("/es/start", s.EsStart)
	mux.HandleFunc("/es/stop", s.EsStop)
	mux.HandleFunc("/es/kill", s.EsKill)
	mux.HandleFunc("/es/status", s.EsStatus)

	if cfg.EnableKeystoreUpdater {
		mux.HandleFunc("/keystore/status", s.KeystoreStatus)
	}

	if cfg.EnableExpVars {
		mux.Handle("/debug/vars", expvar.Handler())
	}

	if cfg.EnableProfiler {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

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
				errMsg := "failed to start the HTTP server"
				log.Error(err, errMsg)
				Exit(errMsg, 1)
			}
		}

	}()
}

// Exit shuts down the HTTP server.
func (s *ProcessServer) Exit() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		log.Error(err, "Fail to stop HTTP server")
		return
	}
}

func (s *ProcessServer) Health(w http.ResponseWriter, req *http.Request) {
	ok(w, "pong")
}

func (s *ProcessServer) EsStart(w http.ResponseWriter, req *http.Request) {
	state, err := s.esProcess.Start(nil)
	if err != nil {
		log.Error(err, "Failed to start es process", "state", state)
		w.WriteHeader(http.StatusInternalServerError)
	}

	writeJson(w, s.esProcess.Status())
}

func (s *ProcessServer) EsStop(w http.ResponseWriter, req *http.Request) {
	state, err := s.esProcess.KillSoft()
	if err != nil {
		log.Error(err, "Failed to stop es process", "state", state)
		w.WriteHeader(http.StatusInternalServerError)
	} else if state == Stopping {
		w.WriteHeader(http.StatusAccepted)
	}

	writeJson(w, s.esProcess.Status())
}

func (s *ProcessServer) EsKill(w http.ResponseWriter, req *http.Request) {
	state, err := s.esProcess.KillHard()
	if err != nil {
		log.Error(err, "Failed to kill es process", "state", state)
		w.WriteHeader(http.StatusInternalServerError)
	} else if state == Killing {
		w.WriteHeader(http.StatusAccepted)
	}

	writeJson(w, s.esProcess.Status())
}

func (s *ProcessServer) EsStatus(w http.ResponseWriter, req *http.Request) {
	writeJson(w, s.esProcess.Status())
}

func (s *ProcessServer) KeystoreStatus(w http.ResponseWriter, req *http.Request) {
	status, err := s.ksUpdater.Status()
	if err != nil {
		ko(w, "Failed to get keystore updater status: "+err.Error())
		return
	}

	writeJson(w, status)
}

// HTTP utilities

func ok(w http.ResponseWriter, msg string) {
	write(w, http.StatusOK, msg)
}

func ko(w http.ResponseWriter, msg string) {
	write(w, http.StatusInternalServerError, fmt.Sprintf(`{"error": "%s"}`, msg))
}

func writeJson(w http.ResponseWriter, obj interface{}) {
	bytes, _ := json.Marshal(obj)
	_, _ = w.Write(bytes)
}

func write(w http.ResponseWriter, statusCode int, msg string) {
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(msg))
}
