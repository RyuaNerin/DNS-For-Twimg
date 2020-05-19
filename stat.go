package main

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

type Stat struct {
	fs *os.File

	HTTPReqeust uint64
	JsonRequest uint64
}

var stat Stat

func (stat *Stat) AddHTTPReqeust() {
	atomic.AddUint64(&stat.HTTPReqeust, 1)
}

func (stat *Stat) AddJsonReqeust() {
	atomic.AddUint64(&stat.JsonRequest, 1)
}

func (stat *Stat) Start() {
	fs, err := os.OpenFile(config.Path.StatLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	stat.fs = fs

	go stat.logger()
}

func (stat *Stat) logger() {
	ltime := time.Now()

	for {
		time.Sleep(time.Until(ltime.Truncate(time.Hour).Add(time.Hour)))

		now := time.Now()

		reqHTTP := atomic.SwapUint64(&stat.HTTPReqeust, 0)
		reqJson := atomic.SwapUint64(&stat.JsonRequest, 0)

		fmt.Fprintf(
			stat.fs,
			"[%s - %s] http: %5d | json : %5d\n",
			ltime.Format("2006-01-02 15:04:05"),
			now.Format("2006-01-02 15:04:05"),
			reqHTTP,
			reqJson,
		)

		ltime = now
	}
}
