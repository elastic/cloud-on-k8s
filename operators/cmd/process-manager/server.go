package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const (
	shutdownTimeout = 5 * time.Second
)

type ProcessServer struct {
	*http.Server
	controller *ProcessController
}

func NewServer(controller *ProcessController) *ProcessServer {
	mux := http.NewServeMux()
	s := ProcessServer{
		&http.Server{
			Addr:    HTTPPort,
			Handler: mux,
		},
		controller,
	}

	mux.HandleFunc("/health", s.Health)
	mux.HandleFunc("/es/start", s.EsStart)
	mux.HandleFunc("/es/stop", s.EsStop)
	mux.HandleFunc("/es/restart", s.EsRestart)
	mux.HandleFunc("/es/kill", s.EsKill)
	mux.HandleFunc("/es/status", s.EsStatus)

	return &s
}

func (s *ProcessServer) Start() {
	go func() {
		if err := s.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				logger.Info("HTTP server closed")
			} else {
				logger.Error(err, "Could not start HTTP server")
			}
		}
		logger.Info("goroutine 'HTTP server' exited")
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
	err := s.controller.Start("es")
	if err != nil {
		ko(w, "es start failed: "+err.Error())
		return
	}

	ok(w, "es started")
}

func (s *ProcessServer) EsStop(w http.ResponseWriter, req *http.Request) {
	err := s.controller.Stop("es", false)
	if err != nil {
		ko(w, "es stop failed: "+err.Error())
		return
	}

	ok(w, "es stopped")
}

func (s *ProcessServer) EsRestart(w http.ResponseWriter, req *http.Request) {
	err := s.controller.Stop("es", true)
	if err != nil {
		ko(w, "es stop failed: "+err.Error())
		return
	}

	err = s.controller.Start("es")
	if err != nil {
		ko(w, "es start failed: "+err.Error())
		return
	}

	ok(w, "es restarted")
}

func (s *ProcessServer) EsStatus(w http.ResponseWriter, req *http.Request) {
	pgid, err := s.controller.Pgid("es")
	if err != nil {
		ko(w, "get pgid failed: "+err.Error())
		return
	}

	ok(w, fmt.Sprintf(`{"pgid":%d}`, pgid))
}

func (s *ProcessServer) EsKill(w http.ResponseWriter, req *http.Request) {
	err := s.controller.HardKill("es")
	if err != nil {
		ko(w, "es kill failed: "+err.Error())
		return
	}

	ok(w, "es killed")
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
