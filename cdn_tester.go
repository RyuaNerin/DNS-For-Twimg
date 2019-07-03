package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"html/template"
	"io"
	"math"
	"net"
	"net/http"
	//"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	//"github.com/garyburd/go-oauth/oauth"
	"github.com/likexian/whois-go"
	"github.com/oschwald/geoip2-golang"
	"github.com/sirupsen/logrus"
	"github.com/sparrc/go-ping"
)

type CdnStatusCollection map[string][]CdnStatus
type CdnStatus struct {
	IP				net.IP			`json:"ip"`
	DefaultCdn		bool			`json:"default_cdn"`
	GeoIP			CdnStatusGeoIP	`json:"geoip"`
	Domain			string			`json:"domain"`
	Organization	string			`json:"organization"`
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
	ct.loadLastJson()

	go ct.loop()
}

func (ct *CDNTester) loop() {
	for {
		nextTime := time.Now().Truncate(config.Test.RefreshInterval.Duration)
		nextTime = nextTime.Add(config.Test.RefreshInterval.Duration)
		
		ct.worker()
		time.Sleep(time.Until(nextTime))
	}
}

func (ct *CDNTester) loadLastJson() {
	fs, err := os.Open(config.Path.TestSave)
	if os.IsNotExist(err) {
		return
	}
	if err != nil && !os.IsNotExist(err) {
		logRusPanic.Error(err)
		return
	}
	defer fs.Close()

	cdnTestResult := make(CdnStatusCollection)
	err = json.NewDecoder(fs).Decode(&cdnTestResult)
	if err != nil {
		logRusPanic.Error(err)
		return
	}

	dnsServer.SetCDN(cdnTestResult)
	ct.setCdnResult(cdnTestResult)
}
func (ct *CDNTester) saveLastJson(cdnTestResult CdnStatusCollection) {
	fs, err := os.OpenFile(config.Path.TestSave, os.O_CREATE | os.O_WRONLY, 644)
	if err != nil {
		logRusPanic.Error(err)
		return
	}
	defer fs.Close()

	fs.Truncate(0)
	fs.Seek(0, 0)

	err = json.NewEncoder(fs).Encode(cdnTestResult)
	if err != nil {
		logRusPanic.Error(err)
		return
	}
}

func (ct *CDNTester) httpIndexHandler(w http.ResponseWriter, r *http.Request) {
	stat.AddHTTPReqeust()

	ct.pageLock.RLock()
	defer ct.pageLock.RUnlock()

	if ct.pageIndex == nil {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("ETag", ct.pageIndexEtag)
		w.Write(ct.pageIndex)
	}
}
func (ct *CDNTester) httpJSONHandler(w http.ResponseWriter, r *http.Request) {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		stat.AddJsonReqeust(net.ParseIP(host))
	}

	ct.pageLock.RLock()
	defer ct.pageLock.RUnlock()

	if ct.pageJSON == nil {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/json")
		w.Header().Set("ETag", ct.pageJSONEtag)
		w.Write(ct.pageJSON)
	}
}

type TemplateData struct {
	UpdatedAt	string					`json:"updated_at"`
	BestCdn		map[string]string		`json:"best_cdn"`
	Detail		CdnStatusCollection		`json:"detail"`
}

func (ct *CDNTester) setCdnResult(cdnTestResult CdnStatusCollection) {
	ct.pageLock.Lock()
	defer ct.pageLock.Unlock()

	ct.saveLastJson(cdnTestResult)

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
				ct.pageIndex		= buff.Bytes()
				ct.pageIndexEtag	= fmt.Sprintf(`"%s"`, hex.EncodeToString(fnv.New64().Sum(ct.pageIndex)))
			}
		}
	}

	// json
	{
		buff := new(bytes.Buffer)
		err := json.NewEncoder(buff).Encode(&data)
		if err == nil {
			ct.pageJSON		= buff.Bytes()
			ct.pageJSONEtag	= fmt.Sprintf(`"%s"`, hex.EncodeToString(fnv.New64().Sum(ct.pageJSON)))
		}
	}
}

func (ct *CDNTester) worker() {
	cdnTestResult := make(CdnStatusCollection)

	logrus.Info("cdn - update")

	var w sync.WaitGroup	
	for _, host := range config.Host {
		w.Add(1)

		go ct.testCdn(&w, host, cdnTestResult)
	}
	w.Wait()

	{
		var sb strings.Builder
		sb.WriteString("cdn - updated\n")
		
		for host, cdn := range cdnTestResult {
			sb.WriteString(fmt.Sprintf("\t%s : %s (Total %d Cdn)\n", host, cdn[0].IP.String(), len(cdn)))
		}

		logrus.Info(sb.String())
	}

	succ := false
	for _, r := range cdnTestResult {
		if len(r) > 0 {
			succ = true
			break
		}
	}

	if !succ {
		return
	}
	
	dnsServer.SetCDN(cdnTestResult)
	ct.setCdnResult(cdnTestResult)

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

func (ct *CDNTester) testCdn(w *sync.WaitGroup, host ConfigHost, m CdnStatusCollection) {
	defer w.Done()
	
	cdnList := make(map[string]*CdnStatus)

	ct.addDefaultCdn(host, cdnList)
	ct.addAdditionalCdn(host, cdnList)
	ct.addCdnListFromThreatCrowd(host, cdnList)
	
	// ping
	ct.parallel(host, cdnList, ct.testPingTask)
	ct.filterCdn(host, cdnList, func(cs CdnStatus) bool { return cs.PingSuccess })

	// country
	ct.getCountry(host, cdnList)

	// arpa
	ct.parallel(host, cdnList, ct.getDomainTask)

	// http-speed
	ct.parallel(host, cdnList, ct.testHTTPTask)
	ct.filterCdn(host, cdnList, func(cs CdnStatus) bool { return cs.HTTPSuccess })

	// whois
	ct.parallel(host, cdnList, ct.getOrganization)


	cdnArray := make([]CdnStatus, 0, len(cdnList))
	for _, r := range cdnList {
		cdnArray = append(cdnArray, *r)
	}

	if len(cdnArray) > 0 {
		sort.Slice(cdnArray, func(i, k int) bool { return cdnArray[i].HTTP.BpsAvg > cdnArray[k].HTTP.BpsAvg })

		m[host.Host] = cdnArray
	}
}

func (ct *CDNTester) filterCdn(host ConfigHost, cdnList map[string]*CdnStatus, skip func(cs CdnStatus) bool) {
	for host, status := range cdnList {
		if !skip(*status) {
			delete(cdnList, host)
		}
	}
}

func (ct *CDNTester) addDefaultCdn(host ConfigHost, cdnList map[string]*CdnStatus) {
	addr, err := defaultDNSResolver.Resolve(host.Host)
	if err == nil && addr != nil {
		cdnList[addr.String()] = &CdnStatus {
			IP			: addr,
			DefaultCdn	: true,
		}
	}

	return
}

func (ct *CDNTester) addCdnListFromThreatCrowd(host ConfigHost, cdnList map[string]*CdnStatus) {
	hres, err := http.Get("https://www.threatcrowd.org/searchApi/v2/domain/report/?domain=" + host.Host)
	if err != nil {
		logRusPanic.Error(err)
		return
	}
	defer hres.Body.Close()

	var res threatCrowdAPIResult
	err = json.NewDecoder(hres.Body).Decode(&res)
	if err != nil {
		logRusPanic.Error(err)
		return
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
			if _, ok := cdnList[ipstr]; !ok {
				cdnList[ipstr] = &CdnStatus {
					IP : ip,
				}
			}
		}
	}

	return
}

func (ct *CDNTester) addAdditionalCdn(host ConfigHost, cdnList map[string]*CdnStatus) {
	for _, addr := range host.CDN {
		ip := net.ParseIP(addr)

		if ip == nil {
			var err error
			ip, err = defaultDNSResolver.Resolve(addr)
			if err != nil {
				continue
			}
		}

		if _, exists := cdnList[ip.String()]; !exists {
			cdnList[ip.String()] = &CdnStatus {
				IP : ip,
			}
		}
	}
}

func (ct *CDNTester) parallel(host ConfigHost, cdnList map[string]*CdnStatus, task func(w *sync.WaitGroup, host ConfigHost, cdn *CdnStatus)) {
	var w sync.WaitGroup
	w.Add(len(cdnList))

	for ip := range cdnList {
		go task(&w, host, cdnList[ip])
	}

	w.Wait()
}

func (ct *CDNTester) testPingTask(w *sync.WaitGroup, host ConfigHost, cdn *CdnStatus) {
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

func (ct *CDNTester) getCountry(host ConfigHost, cdnList map[string]*CdnStatus) {
    db, err := geoip2.Open(config.Path.GeoIP2)
    if err != nil {
		logRusPanic.Error(err)
		return
    }
	defer db.Close()
	
	for _, status := range cdnList {
		city, err := db.City(status.IP)
		if err != nil {
			continue
		}

		status.GeoIP.Country	= city.Country.Names["en"]
		status.GeoIP.City		= city.City.Names["en"]
	}
}

func (ct *CDNTester) getDomainTask(w *sync.WaitGroup, host ConfigHost, cdn *CdnStatus) {
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

func (ct *CDNTester) getOrganization(w *sync.WaitGroup, host ConfigHost, cdn *CdnStatus) {
	defer w.Done()

	result, err := whois.Whois(cdn.IP.String())
	if err != nil {
		return
	}

	for _, reg := range config.DNS.OrganizationRegex {
		matches := reg.FindAllStringSubmatch(result, -1)
		if matches == nil || len(matches) == 0 {
			continue
		}

		for _, match := range matches {
			org := strings.TrimSpace(match[1])

			pass := false
			for _, v := range config.DNS.OrganizationIgnore {
				if strings.EqualFold(org, v) {
					pass = true
					break
				}
			}

			if pass {
				continue
			}
			
			cdn.Organization = org
			return
		}
	}
}

func (ct *CDNTester) testHTTPTask(w *sync.WaitGroup, host ConfigHost, cdn *CdnStatus) {
	defer w.Done()

	client := http.Client {
		Timeout   : config.Test.HTTPTimeout.Duration,
		Transport : &http.Transport {
			Dial				: func(network, addr string) (net.Conn, error) { return net.Dial(network, strings.ReplaceAll(addr, host.Host, cdn.IP.String())) },
			DisableKeepAlives	: true,
		},
	}

	var speeds 		[]float64EIC
	var totalSize 	int
	var totalSec	float64

	buff := make([]byte, config.Test.HTTPBufferSize)
	for i := 0; i < config.Test.HTTPCount; i++ {
		for _, test := range host.Test {
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