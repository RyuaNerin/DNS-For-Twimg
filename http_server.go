package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"html/template"
	"net/http"
	"sync"
	"time"
)

const (
	templatePath = "template/index.html"
	templateBufferSize = 32 * 1024
)

var (
	indexLock sync.RWMutex
	indexData []byte
	indexETag string
)

func httpHandler(w http.ResponseWriter, r *http.Request) {
	indexLock.RLock()
	defer indexLock.RUnlock()

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("ETag", indexETag)
	w.Write(indexData)
}

type TemplateData struct {
	CdnDefault CdnStatus
	CdnList []CdnStatus
	Now string
}

func setCdnResults(cdnDefault CdnStatus, cdnList []CdnStatus) {
	indexLock.Lock()
	defer indexLock.Unlock()

	t, err := template.ParseFiles(templatePath)
	if err != nil {
		return
	}

	data := &TemplateData {
		CdnDefault: cdnDefault,
		CdnList : cdnList,
		Now : time.Now().Format("2006-01-02 15:04 (-0700 MST)"),
	}

	buff := bytes.NewBuffer(make([]byte, 0, templateBufferSize))
	err = t.Execute(buff, data)
	if err != nil {
		return
	}

	indexData = buff.Bytes()

	hash := fnv.New64()
	indexETag = fmt.Sprintf(`"%s"`, hex.EncodeToString(hash.Sum(indexData)))
}