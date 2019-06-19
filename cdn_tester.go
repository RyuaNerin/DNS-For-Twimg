package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/oschwald/geoip2-golang"
	"github.com/sparrc/go-ping"
)

const (
	geoLightPath	= "./GeoLite2-City.mmdb"

	twimgHostName 	= "pbs.twimg.com"
	twimgHostNameD  = "pbs.twimg.com."
	twimgTestURI	= "https://pbs.twimg.com/media/CgAc2lSUMAA30oE.jpg:orig"

	pingCount		= 20
	pingTimeout		= 5000
	httpBufferSize	= 32 * 1024
	httpCount		= 50
	httpTimeout		= 10 * 1000
)

type cdnTester struct {
	cdnMap		map[string]*CdnStatus
	CdnDefualt	CdnStatus
	CdnList		[]CdnStatus
}

type CdnStatus struct {
	IP			net.IP
	IsDefault	bool
	Success		bool
	Location	string
	Domain		string
	Ping		float64Formatted
	HTTPSpeed	float64FormattedByEIC
}

type float64Formatted float64
func (f float64Formatted) String() string {
	return humanize.Commaf(math.Floor(float64(f)) / 100.0)
}
type float64FormattedByEIC float64
func (f float64FormattedByEIC) String() string {
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

func (ct *cdnTester) TestCdn() bool {
	ct.cdnMap = make(map[string]*CdnStatus)

	ct.getDefaultCdn()
	ct.getCdnListFromThreatCrowd()
	
	// ping
	ct.parallel(ct.testPing)
	//ct.filterCdn()

	// country
	ct.getCountry()

	// arpa
	ct.parallel(ct.getDomain)

	// http-speed
	ct.parallel(ct.testHTTP)
	ct.filterCdn()

	// sort by http-speed
	for _, status := range ct.cdnMap {
		if status.IsDefault {
			ct.CdnDefualt = *status
		}
		
		ct.CdnList = append(ct.CdnList, *status)
	}

	sort.Slice(ct.CdnList, func (a, b int) bool { return ct.CdnList[a].HTTPSpeed > ct.CdnList[b].HTTPSpeed })

	return len(ct.CdnList) > 0
}

func (ct *cdnTester) filterCdn() {
	var removal []string
	for ip, status := range ct.cdnMap {
		if !status.Success {
			removal = append(removal, ip)
		}
	}

	for _, ip := range removal {
		delete(ct.cdnMap, ip)
	}
}

func (ct *cdnTester) getDefaultCdn() {
	addr, err := net.ResolveIPAddr("ip", twimgHostName)
	if err == nil {
		ip := addr.IP

		ct.cdnMap[ip.String()] = &CdnStatus {
			IP : ip,
			IsDefault : true,
		}
	}
}

func (ct *cdnTester) getCdnListFromThreatCrowd() {
	hres, err := http.Get("https://www.threatcrowd.org/searchApi/v2/domain/report/?domain=pbs.twimg.com")
	if err != nil {
		panic(err)
	}
	defer hres.Body.Close()

	var res threatCrowdAPIResult
	err = json.NewDecoder(hres.Body).Decode(&res)
	if err != nil {
		panic(err)
	}

	// 1년
	minDate := time.Now().Add((time.Duration)(-365 * 24 * 3) * time.Hour)

	for _, resolution := range res.Resolutions {
		lastResolved, err := time.Parse("2006-01-02", resolution.LastResolved)
		if err != nil {
			continue
		}

		if lastResolved.Before(minDate) {
			continue
		}

		ip := net.ParseIP(resolution.IPAdddress)
		if ip.To4() != nil {
			ipstr := ip.String()
			if _, ok := ct.cdnMap[ipstr]; !ok {
				ct.cdnMap[ipstr] = &CdnStatus {
					IP : ip,
					IsDefault : false,
				}
			}
		}
	}

	return
}

func (ct *cdnTester) parallel(task func(w *sync.WaitGroup, cdn *CdnStatus)) {
	var w sync.WaitGroup
	w.Add(len(ct.cdnMap))

	for ip := range ct.cdnMap {
		go task(&w, ct.cdnMap[ip])
	}

	w.Wait()
}

func (ct *cdnTester) getCountry() {
    db, err := geoip2.Open(geoLightPath)
    if err != nil {
		panic(err)
    }
	defer db.Close()
	
	for _, status := range ct.cdnMap {
		city, err := db.City(status.IP)
		if err != nil {
			continue
		}

		status.Location = fmt.Sprintf("%s - %s", city.Country.Names["en"], city.City.Names["en"])
	}
}

func (ct *cdnTester) testPing(w *sync.WaitGroup, cdn *CdnStatus) {
	defer w.Done()

	pinger, err := ping.NewPinger(cdn.IP.String())
	if err != nil {
		return
	}
	pinger.SetPrivileged(true)
	
	pinger.Count = pingCount
	pinger.Debug = true
	pinger.Timeout = pingTimeout
	pinger.Run()

	stat := pinger.Statistics()

	if stat.PacketsSent != stat.PacketsRecv {
		return
	}

	cdn.Success = true
	cdn.Ping = float64Formatted(float64(stat.AvgRtt) / float64(time.Millisecond))
}

func (ct *cdnTester) getDomain(w *sync.WaitGroup, cdn *CdnStatus) {
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

func (ct *cdnTester) testHTTP(w *sync.WaitGroup, cdn *CdnStatus) {
	defer w.Done()

	cdn.Success = false

	client := http.Client {
		Timeout   : httpTimeout * time.Second,
		Transport : &http.Transport {
			Dial				: func(network, addr string) (net.Conn, error) { return net.Dial(network, strings.ReplaceAll(addr, twimgHostName, cdn.IP.String())) },
			DisableKeepAlives	: true,
		},
	}

	var totalSize int64
	var totalTime float64

	var start time.Time

	buff := make([]byte, httpBufferSize)
	for i := 0; i < httpCount; i++ {
		hreq, err := http.NewRequest("GET", twimgTestURI, nil)
		if err != nil {
			return
		}
		hreq.Close = true

		hres, err := client.Do(hreq)
		if err != nil {
			return
		}
		defer hres.Body.Close()

		if !strings.HasPrefix(hres.Header.Get("content-type"), "image") {
			return
		}
		
		var sz int64
		start = time.Now()
		for {
			read, err := hres.Body.Read(buff)
			if err != nil && err != io.EOF {
				return
			}
			if read == 0 {
				break
			}
			sz += int64(read)
		}

		if hres.ContentLength == 0 {
			totalSize += sz
		} else {
			totalSize += hres.ContentLength
		}

		totalTime += time.Now().Sub(start).Seconds()
	}

	cdn.Success = true
	cdn.HTTPSpeed = float64FormattedByEIC(float64(totalSize) / float64(totalTime))
}