package common

import (
	"net/http"

	"github.com/getsentry/sentry-go"
)

func init() {
	sentry.Init(sentry.ClientOptions{
		Dsn: "https://ae856a34bff542139b853e740033d731@sentry.ryuar.in/18",
		HTTPClient: &http.Client{
			Transport: &http.Transport{},
		},
	})
}
