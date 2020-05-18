package main

import (
	"net"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/vaegt/go-traceroute"
)

type IPLocationResolver struct {
	lock             sync.RWMutex
	geoip2           *geoip2.Reader
	countryCache     map[string]*IPLocationCache
	countryCacheLock sync.Mutex
}

type IPLocation struct {
	Country string
	City    string
}
type IPLocationCache struct {
	Lock     sync.Mutex
	IP       net.IP
	Location IPLocation
}

var ipLocation = IPLocationResolver{
	countryCache: make(map[string]*IPLocationCache),
}

func (ir *IPLocationResolver) Open() {
	ir.lock.Lock()
	defer ir.lock.Unlock()

	if ir.geoip2 != nil {
		err := ir.geoip2.Close()
		if err != nil {
			logRusPanic.Error(err)
			return
		}
	}

	db, err := geoip2.Open(config.Path.GeoIP2)
	if err != nil {
		logRusPanic.Error(err)
		return
	}
	ir.geoip2 = db
}

func (ir *IPLocationResolver) GetRealGeoIP(ip net.IP) IPLocation {
	ir.lock.RLock()
	defer ir.lock.RUnlock()

	ir.countryCacheLock.Lock()
	cache, ok := ir.countryCache[ip.String()]
	if ok {
		ir.countryCacheLock.Unlock()

		cache.Lock.Lock()
		cache.Lock.Unlock()
		return cache.Location
	} else {
		cache = &IPLocationCache{
			IP: ip,
		}
		cache.Lock.Lock()

		ir.countryCache[ip.String()] = cache
	}
	ir.countryCacheLock.Unlock()

	defer cache.Lock.Unlock()

	traceData := traceroute.Exec(ip, 2*time.Second, 1, 30, "icmp", 0)

	err := traceData.All()
	if err != nil {
		return cache.Location
	}

	hops := traceData.Hops[0]
	if len(hops) > 1 {
		for i := len(hops) - 2; i > 0; i-- {
			if ir.getGeoIP(cache, hops[i].AddrIP) {
				return cache.Location
			}
		}

		if ir.getGeoIP(cache, hops[len(hops)-1].AddrIP) {
			return cache.Location
		}
	}

	return cache.Location
}
func (ir *IPLocationResolver) getGeoIP(cache *IPLocationCache, ip net.IP) (ok bool) {
	geoCountry, err := ir.geoip2.City(ip)
	if err != nil {
		return
	}

	if geoCountry.Country.Names["en"] != "" {
		cache.Location.Country = geoCountry.Country.Names["en"]
		cache.Location.City = geoCountry.City.Names["en"]

		ok = true
		return
	}

	return
}
