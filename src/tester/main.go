package tester

import (
	"sync/atomic"
	"time"

	"twimgdns/src/common/cfg"
)

var running int32

func Main() {
	ticker := time.NewTicker(cfg.V.Test.RefreshInterval)

	for {
		go func() {
			if atomic.SwapInt32(&running, 1) != 0 {
				return
			}
			defer atomic.StoreInt32(&running, 0)
			var ct cdnTest
			ct.do()
		}()

		<-ticker.C
	}
}
