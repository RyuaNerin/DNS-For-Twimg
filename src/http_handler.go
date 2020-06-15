package src

import (
	"bytes"
	"encoding/hex"
	"hash/fnv"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	jsoniter "github.com/json-iterator/go"
)

type responseCache struct {
	l sync.RWMutex

	header        map[string]string
	etag          string
	contentLength string

	dataBuff *bytes.Buffer
	data     []byte

	stat *uint64
}

var (
	httpJson = responseCache{
		dataBuff: bytes.NewBuffer(nil),
		stat:     &statJson,
	}
	httpJson2 = responseCache{
		dataBuff: bytes.NewBuffer(nil),
		stat:     &statJson,
	}
)

func (rc *responseCache) Handler(ctx *gin.Context) {
	rc.l.RLock()
	defer rc.l.RUnlock()

	h := ctx.Writer.Header()

	atomic.AddUint64(rc.stat, 1)

	if rc.data == nil {
		ctx.Status(http.StatusNoContent)
	} else {
		h.Set("ETag", rc.etag)
		h.Set("Content-Type", "application/json; charset=utf-8")
		h.Set("Cache-Control", "max-age=300")

		if etag := ctx.GetHeader("If-None-Match"); etag == rc.etag {
			ctx.Status(http.StatusNotModified)
			return
		}

		h.Set("Content-Length", rc.contentLength)

		ctx.Status(http.StatusOK)
		ctx.Writer.Write(rc.data)
	}
}
func (rc *responseCache) update(update func(w io.Writer) error) {
	rc.l.Lock()
	defer rc.l.Unlock()

	h := fnv.New32a()

	rc.dataBuff.Reset()
	if update(io.MultiWriter(h, rc.dataBuff)) == nil {
		rc.data = rc.dataBuff.Bytes()
		rc.etag = hex.EncodeToString(h.Sum(nil))
		rc.contentLength = strconv.Itoa(len(rc.data))
	}
}

func setBestCdn(data testResultV2) {
	for _, r := range data.Detail {
		if r.Best.Addr == "" {
			return
		}
	}

	////////////////////////////////////////////////////////////////////////////////////////////////////

	j := make(testResultV1, len(data.Detail))
	for host, v := range data.Detail {
		var d testResultV1Data
		d.Ip = v.Best.Addr

		d.HTTPSuccess = true
		d.Http.BpsMin = v.Best.Speed
		d.Http.BpsAvg = v.Best.Speed
		d.Http.BpsMax = v.Best.Speed

		d.PingSuccess = true
		d.Ping.RttAvg = v.Best.Ping.Seconds() * 1000
		d.Ping.RttMin = d.Ping.RttAvg
		d.Ping.RttMax = d.Ping.RttAvg

		j[host] = []testResultV1Data{d}
	}
	httpJson.update(
		func(w io.Writer) error {
			return jsoniter.NewEncoder(w).Encode(&j)
		},
	)

	////////////////////////////////////////////////////////////////////////////////////////////////////

	httpJson2.update(
		func(w io.Writer) error {
			return jsoniter.NewEncoder(w).Encode(&data)
		},
	)
}
