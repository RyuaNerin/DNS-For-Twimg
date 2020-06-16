package tester

import (
	"bytes"
	"net/http"
	"time"
	"twimgdns/src/common"
	"twimgdns/src/common/cfg"

	"github.com/getsentry/sentry-go"
	jsoniter "github.com/json-iterator/go"
)

var updateBuffer = bytes.NewBuffer(nil)

func updateServer(data common.Result) {
	updateBuffer.Reset()

	jsoniter.NewEncoder(updateBuffer).Encode(&data)

	for {
		req, _ := http.NewRequest("POST", common.UpdateUri, bytes.NewReader(updateBuffer.Bytes()))
		req.Header.Set(common.UpdateHeaderName, cfg.UpdateHeaderValue)

		res, err := httpClient.Do(req)
		if err == nil && res.StatusCode == http.StatusOK {
			return
		}

		if err != nil {
			sentry.CaptureException(err)
		}

		time.Sleep(5 * time.Second)
	}
}
