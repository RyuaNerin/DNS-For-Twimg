package src

type testResultV1 map[string][]testResultV1Data

type testResultV1Data struct {
	IP         string `json:"ip"`
	DefaultCdn bool   `json:"default_cdn"`
	GeoIP      struct {
		Country string `json:"country"`
		City    string `json:"city"`
	} `json:"geoip"`
	Domain       string `json:"domain"`
	Organization string `json:"organization"`
	Ping         struct {
		Sent int `json:"sent"`
		Recv int `json:"recv"`

		RttMin float64 `json:"rtt_min"`
		RttAvg float64 `json:"rtt_avg"`
		RttMax float64 `json:"rtt_max"`
	} `json:"ping"`
	PingSuccess bool `json:"PingSuccess"`
	HTTP        struct {
		Reqeust  int `json:"reqeust"`
		Response int `json:"response"`

		BpsMin float64 `json:"bps_min"`
		BpsAvg float64 `json:"bps_avg"`
		BpsMax float64 `json:"bps_max"`
	} `json:"http"`
	HTTPSuccess bool `json:"HTTPSuccess"`
}
