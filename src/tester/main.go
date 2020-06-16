package tester

import (
	"time"

	"twimgdns/src/common/cfg"
)

func Main() {
	waitChan := make(chan struct{}, 1)
	waitChan <- struct{}{}

	for {
		var ct cdnTest
		ct.do()

		<-time.After(time.Until(time.Now().Truncate(cfg.V.Test.RefreshInterval).Add(cfg.V.Test.RefreshInterval)))
	}
}
