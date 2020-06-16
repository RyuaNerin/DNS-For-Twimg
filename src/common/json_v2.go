package common

import (
	"time"
)

type Result struct {
	UpdatedAt time.Time             `json:"updated_at"`
	Detail    map[string]ResultData `json:"detail"`
}
type ResultData struct {
	Default ResultDataCdn `json:"default"`
	Best    ResultDataCdn `json:"best"`
}
type ResultDataCdn struct {
	Addr  string        `json:"addr"`
	Ping  time.Duration `json:"ping"`
	Speed float64       `json:"speed"`
}
