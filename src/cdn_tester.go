package src

import (
	"bytes"
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

	"github.com/dustin/go-humanize"
	jsoniter "github.com/json-iterator/go"
	"github.com/miekg/dns"
	"github.com/sparrc/go-ping"
	"golang.org/x/net/http2"
)

var (
	httpClient = http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		},
	}
)

func ip2int(ip net.IP) uint32 {
	if len(ip) == net.IPv4len {
		binary.BigEndian.Uint32(ip)
	}
	return binary.BigEndian.Uint32(ip[len(ip)-4:])
}

func testCdnWorker() {
	waitChan := make(chan struct{}, 1)
	waitChan <- struct{}{}

	for {
		var ct cdnTest
		ct.do()

		<-time.After(time.Until(time.Now().Truncate(config.Test.RefreshInterval).Add(config.Test.RefreshInterval)))
	}
}

type cdnTest struct {
	nameServer    [][]string
	nameServerMap map[uint32]struct{}
}

func (ct *cdnTest) do() {
	ct.nameServerMap = make(map[uint32]struct{})

	result := make(testResultV2, 2)

	for _, namerserverList := range config.DNS.NameServer {
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

	for host, hostInfo := range config.Test.Host {
		td := cdnTestHostData{
			p:            ct,
			host:         host,
			hostInfo:     hostInfo,
			hostTestData: config.Test.TestFile[host],
		}
		config.DNS.Client.Timeout.SetDnsClinet(&td.dnsClient)
		td.do()

		log.Printf("[%s] Best    : %15s / ping : %5.2f ms / http : %7s/s\n", host, td.result.Best.Addr, td.result.Best.Ping.Seconds()*1000, humanize.IBytes(uint64(td.result.Best.Speed)))
		log.Printf("[%s] Cache   : %15s / ping : %5.2f ms / http : %7s/s\n", host, td.result.Cache.Addr, td.result.Cache.Ping.Seconds()*1000, humanize.IBytes(uint64(td.result.Cache.Speed)))
		log.Printf("[%s] Default : %15s / ping : %5.2f ms / http : %7s/s\n", host, td.result.Default.Addr, td.result.Default.Ping.Seconds()*1000, humanize.IBytes(uint64(td.result.Default.Speed)))

		result[host] = td.result
	}

	setBestCdn(result)
	result.save()
}

func (ct *cdnTest) getPublicDNSServerList(url string) {
	r, err := httpClient.Get(url)
	if err != nil {
		return
	}

	var dnsList []struct {
		Ip string `json:"ip"`
	}

	err = jsoniter.NewDecoder(r.Body).Decode(&dnsList)
	if err != nil && err != io.EOF {
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
	hostInfo     *HostInfo
	hostTestData TestDataMap

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
	config.DNS.Client.Timeout.SetDnsClinet(&td.dnsClient)

	//////////////////////////////////////////////////

	if ip, _ := resolve(&td.dnsClient, config.DNS.NameServerDefault, td.host); ip != nil {
		td.cdnAddrList[ip2int(ip)] = &cdnTestHostDataResult{
			addr:       ip.String(),
			nameServer: config.DNS.NameServerDefault,
			isDefault:  true,
		}
	}

	if ip, _ := resolve(&td.dnsClient, config.DNS.NameServerDefault, td.hostInfo.HostCache); ip != nil {
		td.cdnAddrList[ip2int(ip)] = &cdnTestHostDataResult{
			addr:       ip.String(),
			nameServer: config.DNS.NameServerDefault,
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
	chDnsAddr := make(chan []string, config.Test.Worker.Resolve)

	for i := 0; i < config.Test.Worker.Resolve; i++ {
		w.Add(1)
		go func() {
			defer w.Done()

			for dnsAddr := range chDnsAddr {
				ip, err := resolve(&td.dnsClient, dnsAddr, host)
				if err != nil {
					//logV.Printf("[%s] resolve fail : %s %v fail\n", td.host, host, dnsAddr)
					continue
				}

				if ip != nil && ip.To4() != nil {
					//logV.Printf("[%s] resolve succ : %s %v -> %s\n", td.host, host, dnsAddr, ip.String())

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
		return
	}

	minDate := time.Now().Add(config.Test.ThreatCrowdExpire * -1)

	for _, resolution := range jd.Resolutions {
		lastResolved, err := time.Parse("2006-01-02", resolution.LastResolved)
		if err != nil {
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
	chCdnData := make(chan *cdnTestHostDataResult, config.Test.Worker.Ping)

	for i := 0; i < config.Test.Worker.Ping; i++ {
		w.Add(1)
		go func() {
			defer w.Done()

			for cdnData := range chCdnData {
				pinger, err := ping.NewPinger(cdnData.addr)
				if err != nil {
					continue
				}
				pinger.SetPrivileged(true)

				pinger.Timeout = config.Test.PingTimeout
				pinger.Count = config.Test.PingCount

				pinger.Run()

				stats := pinger.Statistics()
				if !cdnData.isDefault && (stats.PacketsRecv != config.Test.PingCount || stats.PacketsSent != config.Test.PingCount) {
					//logV.Printf("[%s] ping abort : %s = r %d / s %d / r %d\n", td.host, cdnData.addr, config.Test.PingCount, stats.PacketsSent, stats.PacketsRecv)
					continue
				}

				//logV.Printf("[%s] ping succ : %s = %.2f\n", td.host, cdnData.addr, stats.AvgRtt.Seconds()*1000)

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
	type testData struct {
		url  string
		hash []byte
	}

	Tf := func(client *http.Client, cdnData *cdnTestHostDataResult) float64 {
		h := sha256.New()

		var tSum float64 = 0
		count := 0

		tr := client.Transport.(*http2.Transport)

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

		for i := 0; i < config.Test.HttpCount && downloaded < config.Test.HttpSpeedSize; i++ {
			rand.Shuffle(len(testDataList), func(i, k int) {
				testDataList[i], testDataList[k] = testDataList[k], testDataList[i]
			})

			for _, d := range testDataList {
				if downloaded >= config.Test.HttpSpeedSize {
					break
				}

				tr.DialTLS = func(network, addr string, cfg *tls.Config) (net.Conn, error) {
					_, port, err := net.SplitHostPort(addr)
					if err != nil {
						return nil, err
					}
					return tls.Dial(network, net.JoinHostPort(cdnData.addr, port), cfg)
				}

				req, err := http.NewRequest("GET", d.url, nil)
				if err != nil {
					//logV.Printf("[%s] http abort : %15s = err\n", td.host, cdnData.addr)
					return 0
				}

				res, err := client.Do(req)
				if err != nil {
					//logV.Printf("[%s] http abort : %15s = err\n", td.host, cdnData.addr)
					return 0
				}

				h.Reset()

				startTime := time.Now()
				wt, err := io.Copy(h, res.Body)
				dt := time.Now().Sub(startTime).Seconds()

				if err != nil && err != io.EOF {
					//logV.Printf("[%s] http abort : %15s = timeout\n", td.host, cdnData.addr)
					res.Body.Close()
					return 0
				}

				if !bytes.Equal(h.Sum(nil), d.hash) {
					//logV.Printf("[%s] http abort : %15s = hash\n", td.host, cdnData.addr)
					return 0
				}

				downloaded += uint64(wt)

				tSum += float64(wt) / dt
				count++
			}
		}

		tSum /= float64(count)

		/**
		logV.Printf(
			"[%s] http succ : %15s = %9s/s (ping: %6.2f) (isDefault: %5v) (isCache: %5v) (%v)\n",
			td.host,
			cdnData.addr,
			humanize.IBytes(uint64(tSum)),
			cdnData.pingAve.Seconds()*1000,
			cdnData.isDefault,
			cdnData.isCache,
			cdnData.nameServer,
		)
		*/

		return tSum
	}

	var w sync.WaitGroup
	chCdnData := make(chan *cdnTestHostDataResult, config.Test.Worker.Http)

	for i := 0; i < config.Test.Worker.Http; i++ {
		w.Add(1)
		go func() {
			defer w.Done()

			client := http.Client{
				Transport: &http2.Transport{
					AllowHTTP: true,
					TLSClientConfig: &tls.Config{
						MinVersion: tls.VersionTLS12,
					},
				},
				Timeout: config.Test.HttpTimeout,
			}

			for cdnData := range chCdnData {
				cdnData.httpAve = Tf(&client, cdnData)
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
