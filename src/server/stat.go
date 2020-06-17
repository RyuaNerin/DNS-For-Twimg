package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"twimgdns/src/common/cfg"
)

var (
	statJson uint64
)

func init() {
	os.MkdirAll(filepath.Dir(cfg.V.Path.StatLog), 0700)

	go func() {
		fs, err := os.OpenFile(cfg.V.Path.StatLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}

		ltime := time.Now().Truncate(time.Hour).Add(time.Hour)

		for {
			time.Sleep(time.Until(ltime))

			reqJson := atomic.SwapUint64(&statJson, 0)

			fmt.Fprintf(
				fs,
				"[%s - %s] json : %6d\n",
				ltime.Format("2006-01-02 15:04:05"),
				time.Now().Format("2006-01-02 15:04:05"),
				reqJson,
			)

			ltime = ltime.Add(time.Hour)
		}
	}()
}
