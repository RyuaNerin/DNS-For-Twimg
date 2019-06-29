package main

import (
	"hash/fnv"
	"bytes"
	"html/template"
	"log"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	//"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	//"github.com/garyburd/go-oauth/oauth"
	"github.com/oschwald/geoip2-golang"
	"github.com/sparrc/go-ping"
)
type CdnStatusCollection map[string][]CdnStatus
type CdnStatus struct {
	IP				net.IP			`json:"ip"`
	DefaultCdn		bool			`json:"default_cdn"`
	GeoIP			CdnStatusGeoIP	`json:"geoip"`
	Domain			string			`json:"domain"`
	Ping			CdnStatusPing	`json:"ping"`
	PingSuccess		bool
	HTTP			CdnStatusHTTP	`json:"http"`
	HTTPSuccess		bool
}
type CdnStatusGeoIP struct {
	Country			string			`json:"country"`
	City			string			`json:"city"`
}
func (g *CdnStatusGeoIP) String() string {
	if g.City != "" {
		return g.Country + " - " + g.City
	}
	return g.Country
}
type CdnStatusPing struct {
	Sent			int				`json:"sent"`
	Recv			int				`json:"recv"`

	RttMin			float64F		`json:"rtt_min"`
	RttAvg			float64F		`json:"rtt_avg"`
	RttMax			float64F		`json:"rtt_max"`
}
type CdnStatusHTTP struct {
	Reqeust			int				`json:"reqeust"`
	Response		int				`json:"response"`

	BpsMin			float64EIC		`json:"bps_min"`
	BpsAvg			float64EIC		`json:"bps_avg"`
	BpsMax			float64EIC		`json:"bps_max"`
}
type cdnStatusTester struct {
	Host		ConfigHost
	cdnList		map[string]*CdnStatus
}

type float64F float64
func (f float64F) String() string {
	return humanize.Commaf(math.Floor(float64(f) * 10) / 10.0)
}
type float64EIC float64
func (f float64EIC) String() string {
	if f < 1000 {
		return fmt.Sprintf("%.1f B/s", f)
	} else if f < 1000 * 1024 {
		return fmt.Sprintf("%.1f KiB/s", f / 1024)
	} else if f < 1000 * 1024 * 1024 {
		return fmt.Sprintf("%.1f MiB/s", f / 1024 / 1024)
	} else {
		return fmt.Sprintf("%.1f GiB/s", f / 1024 / 1024 / 1024)
	}
}

type resolutions struct {
	IPAdddress		string `json:"ip_address"`
	LastResolved	string `json:"last_resolved"`
}
type threatCrowdAPIResult struct {
	Resolutions		[]resolutions `json:"resolutions"`
}

type CDNTester struct {
	pageLock		sync.RWMutex
	pageIndex		[]byte
	pageIndexEtag	string
	pageJSON		[]byte
	pageJSONEtag	string
}

var cdnTester CDNTester

func (ct *CDNTester) Start() {
	for {
		nextTime := time.Now().Truncate(config.Test.RefreshInterval.Duration)
		nextTime = nextTime.Add(config.Test.RefreshInterval.Duration)
		
		time.Sleep(time.Until(nextTime))

		go ct.worker()

	}
}

func (sv *CDNTester) httpIndexHandler(w http.ResponseWriter, r *http.Request) {
	sv.pageLock.RLock()
	defer sv.pageLock.RUnlock()

	if sv.pageIndex == nil {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("ETag", sv.pageIndexEtag)
		w.Write(sv.pageIndex)
	}
}
func (sv *CDNTester) httpJSONHandler(w http.ResponseWriter, r *http.Request) {
	sv.pageLock.RLock()
	defer sv.pageLock.RUnlock()

	if sv.pageJSON == nil {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/json")
		w.Header().Set("ETag", sv.pageJSONEtag)
		w.Write(sv.pageJSON)
	}
}

type TemplateData struct {
	UpdatedAt	string					`json:"updated_at"`
	BestCdn		map[string]string		`json:"best_cdn"`
	Detail		CdnStatusCollection		`json:"detail"`
}

func (sv *CDNTester) setCdnResult(cdnTestResult CdnStatusCollection) {
	sv.pageLock.Lock()
	defer sv.pageLock.Unlock()

	data := TemplateData {
		UpdatedAt	: time.Now().Format("2006-01-02 15:04 (-0700 MST)"),
		Detail		: cdnTestResult,
		BestCdn		: make(map[string]string),
	}
	for host, lst := range cdnTestResult {
		data.BestCdn[host] = lst[0].IP.String()
	}

	// main page
	{
		buff := new(bytes.Buffer)
		t, err := template.ParseFiles(config.HTTP.TemplatePath)
		if err == nil {
			err = t.Execute(buff, &data)
			if err == nil {
				sv.pageIndex		= buff.Bytes()
				sv.pageIndexEtag	= fmt.Sprintf(`"%s"`, hex.EncodeToString(fnv.New64().Sum(sv.pageIndex)))
			}
		}
	}

	// json
	{
		buff := new(bytes.Buffer)
		err := json.NewEncoder(buff).Encode(&data)
		if err == nil {
			sv.pageJSON		= buff.Bytes()
			sv.pageJSONEtag	= fmt.Sprintf(`"%s"`, hex.EncodeToString(fnv.New64().Sum(sv.pageJSON)))
		}
	}
}

func (ct *CDNTester) worker() {
	var cdnTestResult CdnStatusCollection

	if !cdnTestResult.TestCdn() {
		return
	}
	
	dnsServer.SetCDN(cdnTestResult)
	ct.setCdnResult(cdnTestResult)

	{
		var sb strings.Builder
		sb.WriteString("CdnResults Updated\n")
		
		for host, cdn := range cdnTestResult {
			sb.WriteString(fmt.Sprintf("\t%s : %s (Total %d Cdn)\n", host, cdn[0].IP.String(), len(cdn)))
		}

		log.Println(sb.String())
	}

	/*
	oauthClient := oauth.Client {
		Credentials : oauth.Credentials {
			Token : "",
			Secret : "",
		},
		Header : make(http.Header),
	}
	userToken := oauth.Credentials{
		Token: "",
		Secret : "",
	}	
	oauthClient.Header.Set("Accept-Encoding", "gzip, defalte")

	postData := url.Values {}
	postData.Set("status", "")

	resp, err := oauthClient.Post(http.DefaultClient, &userToken, "https://api.twitter.com/1.1/statuses/update.json", postData)
	if err == nil {
		resp.Body.Close()
	}
	*/
}

func (c *CdnStatusCollection) TestCdn() (ok bool) {
	*c = make(CdnStatusCollection)
	for _, host := range config.Host {
		t := cdnStatusTester {
			Host : host,
		}

		lst := t.TestCdn()
		if len(lst) > 0 {
			ok = true
			(*c)[host.Host] = lst
		}
	}

	return
}

func (ct *cdnStatusTester) TestCdn() (cdnList []CdnStatus) {
	ct.cdnList = make(map[string]*CdnStatus)

	ct.getDefaultCdn()
	ct.getCdnListFromThreatCrowd()
	
	// ping
	ct.parallel(ct.testPingTask)
	ct.filterCdn(func(cs CdnStatus) bool { return cs.PingSuccess })

	// country
	ct.getCountry()

	// arpa
	ct.parallel(ct.getDomainTask)

	// http-speed
	ct.parallel(ct.testHTTPTask)
	ct.filterCdn(func(cs CdnStatus) bool { return cs.HTTPSuccess })

	for _, r := range ct.cdnList {
		cdnList = append(cdnList, *r)
	}

	sort.Slice(cdnList, func(i, k int) bool { return cdnList[i].HTTP.BpsAvg > cdnList[k].HTTP.BpsAvg })

	return
}

func (ct *cdnStatusTester) filterCdn(skip func(cs CdnStatus) bool) {
	for host, status := range ct.cdnList {
		if !skip(*status) {
			delete(ct.cdnList, host)
		}
	}
}

func (ct *cdnStatusTester) getDefaultCdn() {
	addr, err := defaultDNSResolver.Resolve(ct.Host.Host)
	if err == nil && addr.String() != "" {
		ct.cdnList[addr.String()] = &CdnStatus {
			IP			: addr,
			DefaultCdn	: true,
		}
	}

	return
}

func (ct *cdnStatusTester) getCdnListFromThreatCrowd() {
	hres, err := http.Get("https://www.threatcrowd.org/searchApi/v2/domain/report/?domain=" + ct.Host.Host)
	if err != nil {
		panic(err)
	}
	defer hres.Body.Close()

	var res threatCrowdAPIResult
	err = json.NewDecoder(hres.Body).Decode(&res)
	if err != nil {
		panic(err)
	}

	minDate := time.Now().Add(config.Test.ThreatCrowdExpire.Duration * -1)

	for _, resolution := range res.Resolutions {
		lastResolved, err := time.Parse("2006-01-02", resolution.LastResolved)
		if err != nil {
			continue
		}

		if lastResolved.Before(minDate) {
			continue
		}

		ip := net.ParseIP(resolution.IPAdddress)
		if ip.To4() != nil && ip.String() != "" {
			ipstr := ip.String()
			if _, ok := ct.cdnList[ipstr]; !ok {
				ct.cdnList[ipstr] = &CdnStatus {
					IP : ip,
				}
			}
		}
	}

	return
}

func (ct *cdnStatusTester) parallel(task func(w *sync.WaitGroup, cdn *CdnStatus)) {
	var w sync.WaitGroup
	w.Add(len(ct.cdnList))

	for ip := range ct.cdnList {
		go task(&w, ct.cdnList[ip])
	}

	w.Wait()
}

func (ct *cdnStatusTester) testPingTask(w *sync.WaitGroup, cdn *CdnStatus) {
	defer w.Done()

	pinger, err := ping.NewPinger(cdn.IP.String())
	if err != nil {
		return
	}
	pinger.SetPrivileged(true)
	
	pinger.Count	= config.Test.PingCount
	pinger.Debug	= true
	pinger.Timeout	= config.Test.PingTimeout.Duration
	pinger.Run()

	stat := pinger.Statistics()

	cdn.Ping.Sent = stat.PacketsSent
	cdn.Ping.Recv = stat.PacketsRecv

	cdn.Ping.RttMin = float64F(float64(stat.MinRtt) / float64(time.Millisecond))
	cdn.Ping.RttAvg = float64F(float64(stat.AvgRtt) / float64(time.Millisecond))
	cdn.Ping.RttMax = float64F(float64(stat.MaxRtt) / float64(time.Millisecond))

	cdn.PingSuccess = stat.PacketsRecv > 0
}

func (ct *cdnStatusTester) getCountry() {
    db, err := geoip2.Open(config.Test.GeoIP2Path)
    if err != nil {
		panic(err)
    }
	defer db.Close()
	
	for _, status := range ct.cdnList {
		city, err := db.City(status.IP)
		if err != nil {
			continue
		}

		status.GeoIP.Country	= city.Country.Names["en"]
		status.GeoIP.City		= city.City.Names["en"]
	}
}

func (ct *cdnStatusTester) getDomainTask(w *sync.WaitGroup, cdn *CdnStatus) {
	defer w.Done()

	names, err := net.LookupAddr(cdn.IP.String())
	if err != nil {
		return
	}

	for _, name := range names {
		if name != "" {
			cdn.Domain = names[0]
			return
		}
	}
}

func (ct *cdnStatusTester) testHTTPTask(w *sync.WaitGroup, cdn *CdnStatus) {
	defer w.Done()

	client := http.Client {
		Timeout   : config.Test.HTTPTimeout.Duration,
		Transport : &http.Transport {
			Dial				: func(network, addr string) (net.Conn, error) { return net.Dial(network, strings.ReplaceAll(addr, ct.Host.Host, cdn.IP.String())) },
			DisableKeepAlives	: true,
		},
	}

	var speeds 		[]float64EIC
	var totalSize 	int
	var totalSec	float64

	buff := make([]byte, config.Test.HTTPBufferSize)
	for i := 0; i < config.Test.HTTPCount; i++ {
		for _, test := range ct.Host.Test {
			cdn.HTTP.Reqeust++
			hreq, err := http.NewRequest("GET", test.URL, nil)
			if err != nil {
				return
			}
			hreq.Close = true

			hres, err := client.Do(hreq)
			if err != nil {
				return
			}
			defer hres.Body.Close()

			h := sha1.New()

			read := 0
			sz := 0
			start := time.Now()
			for {
				read, err = hres.Body.Read(buff)
				if err != nil && err != io.EOF {
					break
				}
				if read == 0 {
					break
				}
				h.Write(buff[:read])
				sz += read
			}
			secs := time.Now().Sub(start).Seconds()

			if (err == nil || err == io.EOF) && hex.EncodeToString(h.Sum(nil)) == test.SHA1 {
				cdn.HTTP.Response++
				
				totalSize += sz
				totalSec += secs

				speeds = append(speeds, float64EIC(float64(sz) / secs))
			}
		}
	}

	cdn.HTTPSuccess = len(speeds) > 0
	
	if len(speeds) > 0 {
		cdn.HTTP.BpsAvg = float64EIC(float64(totalSize) / totalSec)
		cdn.HTTP.BpsMin = speeds[0]
		cdn.HTTP.BpsMax = speeds[0]

		for _, spd := range speeds {
			if spd < cdn.HTTP.BpsMin {
				cdn.HTTP.BpsMin = spd
			}
			if cdn.HTTP.BpsMax < spd {
				cdn.HTTP.BpsMax = spd
			}
		}
	}
}