package main

import (
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
)

type HTTPServer struct {
	serverMux *http.ServeMux
	server    http.Server
}

var httpServer HTTPServer

func (sv *HTTPServer) Start() {

	if sv.serverMux == nil {
		sv.serverMux = http.NewServeMux()

		sv.serverMux.Handle("/debug/pprof/", http.DefaultServeMux)

		sv.serverMux.Handle("/resources/", http.FileServer(http.Dir("resources")))
		sv.serverMux.Handle("/json", http.HandlerFunc(cdnTester.httpJSONHandler))
		sv.serverMux.Handle("/", http.HandlerFunc(cdnTester.httpIndexHandler))

	}

	sv.server = http.Server{
		ErrorLog:     log.New(ioutil.Discard, "", 0),
		Handler:      sv.serverMux,
		ReadTimeout:  config.HTTP.TimeoutRead.Duration,
		WriteTimeout: config.HTTP.TimeoutWrite.Duration,
		IdleTimeout:  config.HTTP.TimeoutIdle.Duration,
	}

	listener, err := net.Listen(config.HTTP.Type, config.HTTP.Listen)
	if err != nil {
		panic(err)
	}

	go func() {
		err = sv.server.Serve(listener)
		if err != nil {
			panic(err)
		}
	}()
}

func (sv *HTTPServer) Restart() {
	err := sv.server.Shutdown(nil)
	if err != nil {
		panic(err)
	}

	sv.Start()
}
