package src

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"twimgdns/src/cfg"
)

var (
	statIndex uint64
	statJson  uint64
)

func init() {
	os.MkdirAll(filepath.Dir(cfg.V.Path.StatLog), 0700)

	fs, err := os.OpenFile(cfg.V.Path.StatLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	ltime := time.Now()

	for {
		time.Sleep(time.Until(ltime.Truncate(time.Hour).Add(time.Hour)))

		now := time.Now()

		reqHTTP := atomic.SwapUint64(&statIndex, 0)
		reqJson := atomic.SwapUint64(&statJson, 0)

		fmt.Fprintf(
			fs,
			"[%s - %s] http: %6d | json : %6d\n",
			ltime.Format("2006-01-02 15:04:05"),
			now.Format("2006-01-02 15:04:05"),
			reqHTTP,
			reqJson,
		)

		ltime = now
	}
}
