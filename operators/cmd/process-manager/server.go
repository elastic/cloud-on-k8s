// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"context"
	_ "expvar"
	"fmt"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates/cert-initializer"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"time"
)

const (
	shutdownTimeout = 5 * time.Second
	HTTPPort        = ":8080"
)

// ProcessServer is an HTTP server with a process controller.
type ProcessServer struct {
	*http.Server
	esProcess *Process
	certInit  *certinitializer.CertInitializer
}

func NewServer(process *Process, certInit *certinitializer.CertInitializer) *ProcessServer {
	mux := http.DefaultServeMux
	s := ProcessServer{
		&http.Server{
			Addr:    HTTPPort,
			Handler: mux,
		},
		process,
		certInit,
	}

	mux.HandleFunc("/health", s.Health)
	mux.HandleFunc("/es/start", s.EsStart)
	mux.HandleFunc("/es/stop", s.EsStop)
	mux.HandleFunc("/es/status", s.EsStatus)
	mux.HandleFunc("/keystore/status", s.EsStatus)
	mux.HandleFunc(certificates.CertInitializerRoute, s.ServeCSR)

	return &s
}

func (s *ProcessServer) Start() {
	go func() {
		if err := s.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				logger.Info("HTTP server closed")
			} else {
				logger.Error(err, "Could not start HTTP server")
				fatal("Could not start HTTP server", err)
			}
		}
		return
	}()
}

func (s *ProcessServer) Stop() {
	ctx, _ := context.WithTimeout(context.Background(), shutdownTimeout)
	if err := s.Shutdown(ctx); err != nil {
		logger.Error(err, "Fail to stop HTTP server")
	}
	logger.Info("HTTP server stopped")
}

func (s *ProcessServer) Health(w http.ResponseWriter, req *http.Request) {
	ok(w, "ping")
}

func (s *ProcessServer) EsStart(w http.ResponseWriter, req *http.Request) {
	msg, err := s.esProcess.Start()
	if err != nil {
		ko(w, fmt.Sprintf("%s: %s", msg, err.Error()))
		return
	}

	ok(w, msg)
}

func (s *ProcessServer) EsStop(w http.ResponseWriter, req *http.Request) {
	var err error

	killHard := false
	hardParam := req.URL.Query().Get("hard")
	if hardParam != "" {
		killHard, err = strconv.ParseBool(hardParam)
		if err != nil {
			logger.Error(err, "Fail to stop")
			ko(w, "Invalid `hard` query parameter")
			return
		}
	}

	/*alwaysHardKill := false
	alwaysHardKillParam := req.URL.Query().Get("safe")
	if alwaysHardKillParam != "" {
		alwaysHardKill, err = strconv.ParseBool(alwaysHardKillParam)
		if err != nil {
			logger.Error(err, "Fail to stop")
			ko(w, "Invalid `always` query parameter")
			return
		}
	}*/

	killHardTimeout := 0
	timeoutParam := req.URL.Query().Get("timeout")
	if timeoutParam != "" {
		hardKillTimeoutSeconds, err := strconv.Atoi(timeoutParam)
		if err != nil {
			logger.Error(err, "Fail to stop")
			ko(w, "Invalid `timeout` query parameter")
			return
		}
		if hardKillTimeoutSeconds < 0 {
			logger.Error(err, "Fail to stop")
			ko(w, "Invalid `timeout` query parameter, must be greater than 0.")
			return
		}
		killHardTimeout = hardKillTimeoutSeconds
	}

	msg, err := s.esProcess.Stop(killHard, time.Duration(killHardTimeout)*time.Second)
	if err != nil {
		logger.Info(msg, "err", err.Error())
		ko(w, fmt.Sprintf("%s", msg))
		return
	}

	ok(w, msg)
}

func (s *ProcessServer) EsStatus(w http.ResponseWriter, req *http.Request) {
	status, err := s.esProcess.Status()
	if err != nil {
		ko(w, "Fail to get status: "+err.Error())
		return
	}

	ok(w, fmt.Sprintf(`{"status":"%s"}`, status))
}

func (s *ProcessServer) ServeCSR(w http.ResponseWriter, r *http.Request) {
	if s.certInit.Terminated {
		ko(w, "CSR already served")
		return
	}
	logger.Info("CSR request")
	if _, err := w.Write(s.certInit.CSR); err != nil {
		logger.Error(err, "Failed to write CSR to the HTTP response")
		ko(w, "Fail to serve CSR")
		return
	}
	return
}

// HTTP utilities

func ok(w http.ResponseWriter, msg string) {
	//logger.Info("HTTP response", "status", "Ok", "msg", msg)
	write(w, http.StatusOK, msg)
}

func ko(w http.ResponseWriter, msg string) {
	//logger.Info("HTTP response", "status", "Error", "msg", msg)
	write(w, http.StatusInternalServerError, msg)
}

func write(w http.ResponseWriter, statusCode int, msg string) {
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(msg))
}
