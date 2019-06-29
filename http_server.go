package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type HTTPServer struct {
	serverMux		http.ServeMux
	server			http.Server

	pageLock		sync.RWMutex
	pageIndex		[]byte
	pageIndexEtag	string
	pageJSON		[]byte
	pageJSONEtag	string
}

var httpServer HTTPServer

func (sv *HTTPServer) Start() {
	sv.serverMux.Handle("/resources/"	, http.FileServer(http.Dir("resources")))
	sv.serverMux.Handle("/json"			, http.HandlerFunc(sv.httpJSONHandler))
	sv.serverMux.Handle("/"				, http.HandlerFunc(sv.httpIndexHandler))

	sv.server = http.Server {
		ErrorLog		: log.New(ioutil.Discard, "", 0),
		Handler			: &sv.serverMux,
		ReadTimeout		: config.HTTP.TimeoutRead .Duration,
		WriteTimeout	: config.HTTP.TimeoutWrite.Duration,
		IdleTimeout		: config.HTTP.TimeoutIdle .Duration,
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


func (sv *HTTPServer) httpIndexHandler(w http.ResponseWriter, r *http.Request) {
	sv.pageLock.RLock()
	defer sv.pageLock.RUnlock()

	if sv.pageIndex == nil {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("ETag", sv.pageIndexEtag)
		w.Write(sv.pageIndex)
	}
}
func (sv *HTTPServer) httpJSONHandler(w http.ResponseWriter, r *http.Request) {
	sv.pageLock.RLock()
	defer sv.pageLock.RUnlock()

	if sv.pageJSON == nil {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/json")
		w.Header().Set("ETag", sv.pageJSONEtag)
		w.Write(sv.pageJSON)
	}
}

type TemplateData struct {
	UpdatedAt	string					`json:"updated_at"`
	BestCdn		map[string]string		`json:"best_cdn"`
	Detail		CdnStatusCollection		`json:"detail"`
}

func (sv *HTTPServer) SetCdnInfomation(cdnTestResult CdnStatusCollection) {
	sv.pageLock.Lock()
	defer sv.pageLock.Unlock()

	data := TemplateData {
		UpdatedAt	: time.Now().Format("2006-01-02 15:04 (-0700 MST)"),
		Detail		: cdnTestResult,
		BestCdn		: make(map[string]string),
	}
	for host, lst := range cdnTestResult {
		data.BestCdn[host] = lst[0].IP.String()
	}

	// main page
	{
		buff := new(bytes.Buffer)
		t, err := template.ParseFiles(config.HTTP.TemplatePath)
		if err == nil {
			err = t.Execute(buff, &data)
			if err == nil {
				sv.pageIndex		= buff.Bytes()
				sv.pageIndexEtag	= fmt.Sprintf(`"%s"`, hex.EncodeToString(fnv.New64().Sum(sv.pageIndex)))
			}
		}
	}

	// json
	{
		buff := new(bytes.Buffer)
		err := json.NewEncoder(buff).Encode(&data)
		if err == nil {
			sv.pageJSON		= buff.Bytes()
			sv.pageJSONEtag	= fmt.Sprintf(`"%s"`, hex.EncodeToString(fnv.New64().Sum(sv.pageJSON)))
		}
	}
}