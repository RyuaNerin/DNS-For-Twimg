package src

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"twimgdns/src/cfg"

	"github.com/dustin/go-humanize"
	"github.com/getsentry/sentry-go"
	jsoniter "github.com/json-iterator/go"
	"github.com/miekg/dns"
	"github.com/sparrc/go-ping"
)

const (
	cacheCAPath = "./twimg.crt"

	contextHost int = 0xF7ABC620D
)

var (
	httpClient = http.Client{
		Timeout: cfg.V.HTTP.Client.Timeout.Timeout,
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,

			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},

			IdleConnTimeout:       cfg.V.HTTP.Client.Timeout.IdleConnTimeout,
			ExpectContinueTimeout: cfg.V.HTTP.Client.Timeout.ExpectContinueTimeout,
			ResponseHeaderTimeout: cfg.V.HTTP.Client.Timeout.ResponseHeaderTimeout,
			TLSHandshakeTimeout:   cfg.V.HTTP.Client.Timeout.TLSHandshakeTimeout,

			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var d net.Dialer

				v := ctx.Value(contextHost)
				if h, ok := v.(string); ok && h != "" {
					_, port, err := net.SplitHostPort(addr)
					if err == nil {
						return d.DialContext(ctx, network, net.JoinHostPort(h, port))
					}
				}

				return d.DialContext(ctx, network, addr)
			},
		},
	}
)

func init() {
	go func() {
		waitChan := make(chan struct{}, 1)
		waitChan <- struct{}{}

		for {
			var ct cdnTest
			ct.do()

			<-time.After(time.Until(time.Now().Truncate(cfg.V.Test.RefreshInterval).Add(cfg.V.Test.RefreshInterval)))
		}
	}()
}

func ip2int(ip net.IP) uint32 {
	if len(ip) == net.IPv4len {
		binary.BigEndian.Uint32(ip)
	}
	return binary.BigEndian.Uint32(ip[len(ip)-4:])
}

type cdnTest struct {
	nameServer    [][]string
	nameServerMap map[uint32]struct{}
}

func (ct *cdnTest) do() {
	ct.nameServerMap = make(map[uint32]struct{})

	result := make(testResultV2, 2)

	for _, namerserverList := range cfg.V.DNS.NameServer {
		l := make([]string, 0, len(namerserverList))

		for _, nameserver := range namerserverList {
			if ip := net.ParseIP(nameserver); ip != nil && ip.To4() != nil {
				l = append(l, nameserver)
				ct.nameServerMap[ip2int(ip)] = struct{}{}
			}
		}

		ct.nameServer = append(ct.nameServer, namerserverList)
	}

	ct.getPublicDNSServerList("https://public-dns.info/nameserver/kr.json")
	ct.getPublicDNSServerList("https://public-dns.info/nameserver/jp.json")

	logV.Printf("nameserver Count : %d\n", len(ct.nameServer))

	for host, hostInfo := range cfg.V.Test.Host {
		td := cdnTestHostData{
			p:            ct,
			host:         host,
			hostInfo:     hostInfo,
			hostTestData: cfg.V.Test.TestFile[host],
		}
		td.do()

		httpClient.CloseIdleConnections()

		log.Printf("[%s] Best    : %15s / ping : %6.2f ms / http : %7s/s\n", host, td.result.Best.Addr, td.result.Best.Ping.Seconds()*1000, humanize.IBytes(uint64(td.result.Best.Speed)))
		log.Printf("[%s] Cache   : %15s / ping : %6.2f ms / http : %7s/s\n", host, td.result.Cache.Addr, td.result.Cache.Ping.Seconds()*1000, humanize.IBytes(uint64(td.result.Cache.Speed)))
		log.Printf("[%s] Default : %15s / ping : %6.2f ms / http : %7s/s\n", host, td.result.Default.Addr, td.result.Default.Ping.Seconds()*1000, humanize.IBytes(uint64(td.result.Default.Speed)))

		result[host] = td.result
	}

	setBestCdn(result)
	result.save()
}

func (ct *cdnTest) getPublicDNSServerList(url string) {
	r, err := httpClient.Get(url)
	if err != nil {
		sentry.CaptureException(err)
		return
	}

	var dnsList []struct {
		Ip string `json:"ip"`
	}

	err = jsoniter.NewDecoder(r.Body).Decode(&dnsList)
	if err != nil && err != io.EOF {
		sentry.CaptureException(err)
		return
	}

	for _, dns := range dnsList {
		if ip := net.ParseIP(dns.Ip); ip != nil && ip.To4() != nil {
			ipi := ip2int(ip)

			if _, ok := ct.nameServerMap[ipi]; !ok {
				ct.nameServer = append(ct.nameServer, []string{dns.Ip})
				ct.nameServerMap[ipi] = struct{}{}
			}
		}
	}
}

type cdnTestHostData struct {
	p *cdnTest

	host         string
	hostInfo     *cfg.HostInfo
	hostTestData cfg.TestDataMap

	dnsClient dns.Client

	cdnAddrListLock sync.Mutex
	cdnAddrList     map[uint32]*cdnTestHostDataResult

	pingSum int64 // Microseconds

	result testResultData
}

type cdnTestHostDataResult struct {
	addr       string
	nameServer []string

	pingAve time.Duration
	httpAve float64

	isDefault bool
	isCache   bool
}

func (td *cdnTestHostData) do() {
	td.cdnAddrList = make(map[uint32]*cdnTestHostDataResult, 30)
	td.dnsClient = dns.Client{
		Net: "udp",
	}

	//////////////////////////////////////////////////

	if ip, _ := resolve(cfg.V.DNS.NameServerDefault, td.host); ip != nil {
		td.cdnAddrList[ip2int(ip)] = &cdnTestHostDataResult{
			addr:       ip.String(),
			nameServer: cfg.V.DNS.NameServerDefault,
			isDefault:  true,
		}
	}

	if ip, _ := resolve(cfg.V.DNS.NameServerDefault, td.hostInfo.HostCache); ip != nil {
		td.cdnAddrList[ip2int(ip)] = &cdnTestHostDataResult{
			addr:       ip.String(),
			nameServer: cfg.V.DNS.NameServerDefault,
			isCache:    true,
		}
	}

	for _, host := range td.hostInfo.Host {
		td.getCdnAddrFromNameServer(host)
		td.getCdnAddrFromThreatCrowd(host)
	}
	logV.Printf("[%s] cdn count : %d\n", td.host, len(td.cdnAddrList))

	//////////////////////////////////////////////////

	logV.Printf("[%s] ping start\n", td.host)
	td.pingAndFilter()
	logV.Printf("[%s] ping done (%d)\n", td.host, len(td.cdnAddrList))

	logV.Printf("[%s] http start\n", td.host)
	td.httpSpeedTest()
	logV.Printf("[%s] http done (%d)\n", td.host, len(td.cdnAddrList))

	//////////////////////////////////////////////////

	var maxHttpAve float64
	for _, data := range td.cdnAddrList {
		if maxHttpAve < data.httpAve && !data.isCache {
			maxHttpAve = data.httpAve
			td.result.Best = testResultDataCdn{
				Addr:  data.addr,
				Ping:  data.pingAve,
				Speed: data.httpAve,
			}
		}

		if data.isCache {
			td.result.Cache = testResultDataCdn{
				Addr:  data.addr,
				Ping:  data.pingAve,
				Speed: data.httpAve,
			}
		}

		if data.isDefault {
			td.result.Default = testResultDataCdn{
				Addr:  data.addr,
				Ping:  data.pingAve,
				Speed: data.httpAve,
			}
		}
	}
}

func (td *cdnTestHostData) getCdnAddrFromNameServer(host string) {
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		ipi := ip2int(ip)
		if _, ok := td.cdnAddrList[ipi]; !ok {
			td.cdnAddrList[ipi] = &cdnTestHostDataResult{
				addr: ip.String(),
			}
		}
		return
	}

	if !strings.HasSuffix(host, ".") {
		host = host + "."
	}

	var w sync.WaitGroup
	chDnsAddr := make(chan []string, cfg.V.Test.Worker.Resolve)

	for i := 0; i < cfg.V.Test.Worker.Resolve; i++ {
		w.Add(1)
		go func() {
			defer w.Done()

			for dnsAddr := range chDnsAddr {
				ip, ok := resolve(dnsAddr, host)
				if !ok {
					continue
				}

				if ip != nil && ip.To4() != nil {
					ipi := ip2int(ip)

					td.cdnAddrListLock.Lock()
					if _, ok := td.cdnAddrList[ipi]; !ok {
						td.cdnAddrList[ipi] = &cdnTestHostDataResult{
							addr:       ip.String(),
							nameServer: dnsAddr,
						}
					}
					td.cdnAddrListLock.Unlock()
				}
			}
		}()
	}

	for _, addr := range td.p.nameServer {
		chDnsAddr <- addr
	}
	close(chDnsAddr)

	w.Wait()
}

func (td *cdnTestHostData) getCdnAddrFromThreatCrowd(host string) {
	res, err := httpClient.Get("https://www.threatcrowd.org/searchApi/v2/domain/report/?domain=" + host)
	if err != nil {
		sentry.CaptureException(err)
		return
	}
	defer res.Body.Close()

	var jd struct {
		Resolutions []struct {
			IpAdddress   string `json:"ip_address"`
			LastResolved string `json:"last_resolved"`
		} `json:"resolutions"`
	}
	err = jsoniter.NewDecoder(res.Body).Decode(&jd)
	if err != nil {
		sentry.CaptureException(err)
		return
	}

	minDate := time.Now().Add(cfg.V.Test.ThreatCrowdExpire * -1)

	for _, resolution := range jd.Resolutions {
		lastResolved, err := time.Parse("2006-01-02", resolution.LastResolved)
		if err != nil {
			sentry.CaptureException(err)
			continue
		}

		if lastResolved.Before(minDate) {
			continue
		}

		ip := net.ParseIP(resolution.IpAdddress)
		if ip.To4() != nil {
			ipi := ip2int(ip)
			if _, ok := td.cdnAddrList[ipi]; !ok {
				td.cdnAddrList[ipi] = &cdnTestHostDataResult{
					addr:       ip.String(),
					nameServer: []string{"Threat Crowd"},
				}
			}
		}
	}
}

func (td *cdnTestHostData) pingAndFilter() {
	var w sync.WaitGroup
	chCdnData := make(chan *cdnTestHostDataResult, cfg.V.Test.Worker.Ping)

	for i := 0; i < cfg.V.Test.Worker.Ping; i++ {
		w.Add(1)
		go func() {
			defer w.Done()

			for cdnData := range chCdnData {
				pinger, err := ping.NewPinger(cdnData.addr)
				if err != nil {
					sentry.CaptureException(err)
					continue
				}
				pinger.SetPrivileged(true)

				pinger.Timeout = cfg.V.Test.PingTimeout
				pinger.Count = cfg.V.Test.PingCount

				pinger.Run()

				stats := pinger.Statistics()
				if !cdnData.isDefault && (stats.PacketsRecv != cfg.V.Test.PingCount || stats.PacketsSent != cfg.V.Test.PingCount) {
					continue
				}

				cdnData.pingAve = stats.AvgRtt
				atomic.AddInt64(&td.pingSum, stats.AvgRtt.Microseconds())
			}
		}()
	}

	for _, d := range td.cdnAddrList {
		chCdnData <- d
	}
	close(chCdnData)

	w.Wait()

	////////////////////////////////////////////////////////////////////////////////////////////////////
	// 상태 나쁜 CDN 제거
	pingAve := time.Duration(td.pingSum/int64(len(td.cdnAddrList))) * time.Microsecond * 3 / 2

	for k, data := range td.cdnAddrList {
		if data.isDefault || data.isCache {
			continue
		}
		if data.pingAve == 0 || data.pingAve > pingAve {
			delete(td.cdnAddrList, k)
		}
	}
}

func (td *cdnTestHostData) httpSpeedTest() {
	Tf := func(cdnData *cdnTestHostDataResult) float64 {
		type testData struct {
			url  string
			hash []byte
		}

		h := sha256.New()

		var tSum float64 = 0
		count := 0

		var downloaded uint64 = 0

		testDataList := make([]testData, 0, len(td.hostTestData))
		for url, hash := range td.hostTestData {
			testDataList = append(
				testDataList,
				testData{
					url:  url,
					hash: hash,
				},
			)
		}

		for downloaded < cfg.V.Test.HttpSpeedSize {
			d := testDataList[rand.Intn(len(testDataList))]

			h.Reset()

			req, err := http.NewRequestWithContext(
				context.WithValue(context.Background(), contextHost, cdnData.addr),
				"GET",
				d.url,
				nil,
			)
			if err != nil {
				sentry.CaptureException(err)
				return 0
			}

			startTime := time.Now()
			res, err := httpClient.Do(req)
			if err != nil {
				sentry.CaptureException(err)
				if res != nil && res.Body != nil {
					res.Body.Close()
				}
				return 0
			}

			wt, err := io.Copy(h, res.Body)
			dt := time.Now().Sub(startTime).Seconds()

			if err != nil && err != io.EOF {
				sentry.CaptureException(err)
				res.Body.Close()
				return 0
			}

			if !bytes.Equal(h.Sum(nil), d.hash) {
				res.Body.Close()
				return 0
			}

			tSum += float64(wt) / dt
			count++

			downloaded += uint64(wt)
		}

		tSum /= float64(count)

		return tSum
	}

	var w sync.WaitGroup
	chCdnData := make(chan *cdnTestHostDataResult, cfg.V.Test.Worker.Http)

	for i := 0; i < cfg.V.Test.Worker.Http; i++ {
		w.Add(1)
		go func() {
			defer w.Done()

			for cdnData := range chCdnData {
				cdnData.httpAve = Tf(cdnData)
			}
		}()
	}

	for _, cdnData := range td.cdnAddrList {
		chCdnData <- cdnData
	}
	close(chCdnData)

	w.Wait()

	for k, data := range td.cdnAddrList {
		if data.isDefault || data.isCache {
			continue
		}
		if data.httpAve == 0 {
			delete(td.cdnAddrList, k)
		}
	}
}
