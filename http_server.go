package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"html/template"
	"net/http"
	"sync"
	"time"
)

var (
	pageLock		sync.RWMutex
	pageIndex		[]byte
	pageIndexEtag	string
	pageJSON		[]byte
	pageJSONEtag	string
)

func httpIndexHandler(w http.ResponseWriter, r *http.Request) {
	pageLock.RLock()
	defer pageLock.RUnlock()

	if pageIndex == nil {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("ETag", pageIndexEtag)
		w.Write(pageIndex)
	}
}
func httpJSONHandler(w http.ResponseWriter, r *http.Request) {
	pageLock.RLock()
	defer pageLock.RUnlock()

	if pageJSON == nil {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/json")
		w.Header().Set("ETag", pageJSONEtag)
		w.Write(pageJSON)
	}
}

type TemplateData struct {
	UpdatedAt	string					`json:"updated_at"`
	BestCdn		map[string]string		`json:"best_cdn"`
	Detail		CdnStatusCollection		`json:"detail"`
}

func setHTTPPage(cdnTestResult CdnStatusCollection) {
	pageLock.Lock()
	defer pageLock.Unlock()

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
				pageIndex		= buff.Bytes()
				pageIndexEtag	= fmt.Sprintf(`"%s"`, hex.EncodeToString(fnv.New64().Sum(pageIndex)))
			}
		}
	}

	// json
	{
		buff := new(bytes.Buffer)
		err := json.NewEncoder(buff).Encode(&data)
		if err == nil {
			pageJSON		= buff.Bytes()
			pageJSONEtag	= fmt.Sprintf(`"%s"`, hex.EncodeToString(fnv.New64().Sum(pageJSON)))
		}
	}


}