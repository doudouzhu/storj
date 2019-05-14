// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package offersweb

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/zeebo/errs"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Error is satellite referral error type
var Error = errs.Class("satellite referral error")

// Config contains configuration for referral offersweb server
type Config struct {
	Address string `help:"server address of the frontend app" default:"127.0.0.1:8090"`
}

// Server represents referral offersweb server
type Server struct {
	log *zap.Logger

	config Config

	listener net.Listener
	server   http.Server
}

// NewServer creates new instance of offersweb server
func NewServer(logger *zap.Logger, config Config, listener net.Listener) *Server {
	server := Server{
		log:      logger,
		config:   config,
		listener: listener,
	}

	logger.Debug("Starting offersweb UI...")

	mux := http.NewServeMux()

	mux.Handle("/", server.localAccessHandler(server.appHandler))

	server.server = http.Server{
		Handler: mux,
	}

	return &server
}

// localAccessHandler is a method for ensuring allow request only from localhost
func (s *Server) localAccessHandler(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.Split(req.RemoteAddr, ":")[0] != strings.Split(s.config.Address, ":")[0] {
			s.serveError(w, req)
			return
		}
		next(w, req)
	})
}

// appHandler is web app http handler function
func (s *Server) appHandler(w http.ResponseWriter, req *http.Request) {
	//TODO: handle request
}

func (s *Server) serveError(w http.ResponseWriter, req *http.Request) {
	//TODO: serve a 404 page
}

// Run starts the server that host admin web app and api endpoint
func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	var group errgroup.Group
	group.Go(func() error {
		<-ctx.Done()
		return s.server.Shutdown(nil)
	})
	group.Go(func() error {
		defer cancel()
		return s.server.Serve(s.listener)
	})

	return group.Wait()
}

// Close closes server and underlying listener
func (s *Server) Close() error {
	return s.server.Close()
}
