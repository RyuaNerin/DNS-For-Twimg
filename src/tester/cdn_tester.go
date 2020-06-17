package tester

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

	"twimgdns/src/common"
	"twimgdns/src/common/cfg"

	"github.com/asmpro/go-ping"
	"github.com/dustin/go-humanize"
	"github.com/getsentry/sentry-go"
	jsoniter "github.com/json-iterator/go"
	"github.com/miekg/dns"
)

const (
	cacheCAPath = "./twimg.crt"
)

var (
	httpClient = newHttpClient()
)

func newHttpClient() *http.Client {
	return &http.Client{
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
		},
	}
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

	result := common.Result{
		Detail: make(map[string]common.ResultData, 2),
	}

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

	common.Verbose.Printf("nameserver Count : %d\n", len(ct.nameServer))

	for host, hostInfo := range cfg.V.Test.Host {
		td := cdnTestHostData{
			p:            ct,
			host:         host,
			hostList:     hostInfo,
			hostTestData: cfg.TestFile[host],
		}
		td.do()

		log.Printf("[%s] Best    : %15s / ping : %6.2f ms / http : %7s/s\n", host, td.result.Best.Addr, td.result.Best.Ping.Seconds()*1000, humanize.IBytes(uint64(td.result.Best.Speed)))
		log.Printf("[%s] Default : %15s / ping : %6.2f ms / http : %7s/s\n", host, td.result.Default.Addr, td.result.Default.Ping.Seconds()*1000, humanize.IBytes(uint64(td.result.Default.Speed)))

		result.Detail[host] = td.result
	}

	result.UpdatedAt = time.Now()

	go updateServer(result)
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
	hostList     []string
	hostTestData cfg.TestDataMap

	dnsClient dns.Client

	cdnAddrListLock sync.Mutex
	cdnAddrList     map[uint32]*cdnTestHostDataResult

	pingSum      int64 // Microseconds
	pingSumCount int64

	result common.ResultData
}

type cdnTestHostDataResult struct {
	addr       string
	nameServer []string

	pingAve time.Duration
	httpAve float64

	isDefault bool
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

	for _, host := range td.hostList {
		td.getCdnAddrFromNameServer(host)
		td.getCdnAddrFromThreatCrowd(host)
	}
	common.Verbose.Printf("[%s] cdn count : %d\n", td.host, len(td.cdnAddrList))

	//////////////////////////////////////////////////

	common.Verbose.Printf("[%s] ping start\n", td.host)
	td.pingAndFilter()
	common.Verbose.Printf("[%s] ping done (%d)\n", td.host, len(td.cdnAddrList))

	common.Verbose.Printf("[%s] http start\n", td.host)
	td.httpSpeedTest()
	common.Verbose.Printf("[%s] http done (%d)\n", td.host, len(td.cdnAddrList))

	//////////////////////////////////////////////////

	var maxHttpAve float64
	for _, data := range td.cdnAddrList {
		if maxHttpAve < data.httpAve {
			maxHttpAve = data.httpAve
			td.result.Best = common.ResultDataCdn{
				Addr:  data.addr,
				Ping:  data.pingAve,
				Speed: data.httpAve,
			}
		}

		if data.isDefault {
			td.result.Default = common.ResultDataCdn{
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
		if err != nil || lastResolved.Before(minDate) {
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
				pinger, _ := ping.NewPinger(cdnData.addr)
				pinger.Count = cfg.V.Test.PingCount
				pinger.Timeout = cfg.V.Test.PingTimeout

				pinger.SetPrivileged(true)
				pinger.Run()

				stats := pinger.Statistics()
				if !cdnData.isDefault && (stats.PacketsRecv != cfg.V.Test.PingCount || stats.PacketsSent != cfg.V.Test.PingCount) {
					continue
				}

				cdnData.pingAve = stats.AvgRtt
				atomic.AddInt64(&td.pingSum, int64(stats.AvgRtt))
				atomic.AddInt64(&td.pingSumCount, 1)

				common.Verbose.Printf("[%s] ping %15s : %8.2f ms\n", td.host, cdnData.addr, float64(cdnData.pingAve)/float64(time.Millisecond))
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
	pingAve := time.Duration(td.pingSum / int64(td.pingSumCount))

	for k, data := range td.cdnAddrList {
		if !data.isDefault && (data.pingAve == 0 || data.pingAve > pingAve) {
			delete(td.cdnAddrList, k)
			continue
		}
	}
}

func (td *cdnTestHostData) httpSpeedTest() {
	type testData struct {
		url  string
		hash []byte
	}
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

	Tf := func(client *http.Client, cdnData *cdnTestHostDataResult) float64 {
		h := sha256.New()

		tr := client.Transport.(*http.Transport)
		tr.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, _ := net.SplitHostPort(addr)

			var dialer net.Dialer
			c, err := dialer.DialContext(ctx, network, net.JoinHostPort(cdnData.addr, port))
			if err != nil {
				return nil, err
			}

			tconfig := tr.TLSClientConfig.Clone()
			tconfig.ServerName = host

			tc := tls.Client(c, tconfig)

			errChannel := make(chan error, 1)
			go func() {
				errChannel <- tc.Handshake()
			}()
			select {
			case <-ctx.Done():
				tc.Close()
				return nil, http.ErrHandlerTimeout
			case err = <-errChannel:
				if err != nil {
					return nil, err
				}
			}

			go func() {
				errChannel <- tc.VerifyHostname(host)
			}()
			select {
			case <-ctx.Done():
				tc.Close()
				return nil, http.ErrHandlerTimeout
			case err = <-errChannel:
				if err != nil {
					return nil, err
				}
			}

			return tc, nil
		}

		var downloaded uint64 = 0
		startTime := time.Now()

		for downloaded < cfg.V.Test.HttpSpeedSize {
			d := testDataList[rand.Intn(len(testDataList))]

			req, err := http.NewRequest("GET", d.url, nil)
			if err != nil {
				sentry.CaptureException(err)
				return 0
			}

			res, err := client.Do(req)
			if err != nil {
				sentry.CaptureException(err)
				if res != nil && res.Body != nil {
					res.Body.Close()
				}
				return 0
			}

			h.Reset()
			wt, err := io.Copy(h, res.Body)

			if err != nil && err != io.EOF {
				sentry.CaptureException(err)
				res.Body.Close()
				return 0
			}

			if !bytes.Equal(h.Sum(nil), d.hash) {
				res.Body.Close()
				return 0
			}

			downloaded += uint64(wt)
		}

		return float64(downloaded) / time.Since(startTime).Seconds()
	}

	var w sync.WaitGroup
	chCdnData := make(chan *cdnTestHostDataResult, cfg.V.Test.Worker.Http)

	for i := 0; i < cfg.V.Test.Worker.Http; i++ {
		w.Add(1)
		go func() {
			defer w.Done()

			client := newHttpClient()
			var timeout time.Duration

			for cdnData := range chCdnData {
				if cdnData.isDefault {
					timeout = client.Timeout
					client.Timeout = 0
				}
				cdnData.httpAve = Tf(client, cdnData)
				if cdnData.httpAve != 0 {
					common.Verbose.Printf("[%s] http %15s : %8s/s\n", td.host, cdnData.addr, humanize.IBytes(uint64(cdnData.httpAve)))
				}
				if cdnData.isDefault {
					client.Timeout = timeout
				}
			}
		}()
	}

	for _, cdnData := range td.cdnAddrList {
		chCdnData <- cdnData
	}
	close(chCdnData)

	w.Wait()

	for k, data := range td.cdnAddrList {
		if !data.isDefault && (data.httpAve == 0) {
			delete(td.cdnAddrList, k)
			continue
		}
	}
}
