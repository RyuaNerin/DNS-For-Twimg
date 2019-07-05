package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Stat struct {
	fs				*os.File

	DNSRequest		uint64
	HTTPReqeust		uint64
	JsonRequest		uint64

	AddrLock		sync.Mutex
	Addr			map[uint32]struct{}

	AddrDNSLock		sync.Mutex
	AddrDNS			map[uint32]struct{}

	AddrAPILock		sync.Mutex
	AddrAPI			map[uint32]struct{}
}

var stat = Stat {
	AddrDNS : make(map[uint32]struct{}),
	AddrAPI : make(map[uint32]struct{}),
}

func (stat *Stat) Load() {
	stat.loadMap(config.Path.StatSaveDNS, &stat.AddrDNSLock, stat.AddrDNS)
	stat.loadMap(config.Path.StatSaveAPI, &stat.AddrAPILock, stat.AddrAPI)
}
func (stat *Stat) loadMap(path string, lock *sync.Mutex, addr map[uint32]struct{}) {
	lock.Lock()
	defer lock.Unlock()

	fs, err := os.Open(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil && !os.IsNotExist(err) {
		logRusPanic.Error(err)
		return
	}
	defer fs.Close()

	buff := make([]byte, 4)
	for {
		read, err := fs.Read(buff)
		if read == 0 || err == io.EOF {
			break
		}
		if err != nil && err != io.EOF {
			panic(err)
		}

		u := uint32(buff[0]) << 24 | uint32(buff[1]) << 16 | uint32(buff[2]) << 8 | uint32(buff[3])
		
		stat.Addr[u] = struct{}{}
		addr[u] = struct{}{}
	}
}
func (stat *Stat) Save() {
	stat.saveMap(config.Path.StatSaveDNS, &stat.AddrDNSLock, stat.AddrDNS)
	stat.saveMap(config.Path.StatSaveAPI, &stat.AddrAPILock, stat.AddrAPI)
}
func (stat *Stat) saveMap(path string, lock *sync.Mutex, addr map[uint32]struct{}) {
	fs, err := os.OpenFile(path, os.O_CREATE | os.O_WRONLY, 644)
	if err != nil {
		logRusPanic.Error(err)
		return
	}
	defer fs.Close()

	fs.Truncate(0)
	fs.Seek(0, 0)

	buff := make([]byte, 4)
	for u := range addr {
		buff[0] = byte((u >> 24) & 0xFF)
		buff[1] = byte((u >> 16) & 0xFF)
		buff[2] = byte((u >>  8) & 0xFF)
		buff[3] = byte((u      ) & 0xFF)
		fs.Write(buff)
	}
}

func (stat *Stat) AddDNSReqeust(ip net.IP) {
	atomic.AddUint64(&stat.DNSRequest, 1)
	
	u := uint32(ip[12]) << 24 | uint32(ip[13]) << 16 | uint32(ip[14]) << 8 | uint32(ip[15])

	stat.AddrDNSLock.Lock()
	stat.AddrDNS[u] = struct{}{}
	stat.AddrDNSLock.Unlock()

	stat.AddrLock.Lock()
	stat.Addr[u] = struct{}{}
	stat.AddrLock.Unlock()
}

func (stat *Stat) AddHTTPReqeust() {
	atomic.AddUint64(&stat.HTTPReqeust, 1)
}

func (stat *Stat) AddJsonReqeust(ip net.IP) {
	if ip != nil {
		u := uint32(ip[12]) << 24 | uint32(ip[13]) << 16 | uint32(ip[14]) << 8 | uint32(ip[15])

		stat.AddrAPILock.Lock()
		stat.AddrAPI[u] = struct{}{}
		stat.AddrAPILock.Unlock()

		stat.AddrLock.Lock()
		stat.Addr[u] = struct{}{}
		stat.AddrLock.Unlock()
	}

	atomic.AddUint64(&stat.JsonRequest, 1)
}

func (stat *Stat) Start() {
	stat.Load()
	
	fs, err := os.OpenFile(config.Path.StatLog, os.O_CREATE | os.O_APPEND | os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	stat.fs = fs

	go stat.logger()
}

func (stat *Stat) logger() {
	ltime := time.Now()

	for {
		time.Sleep(time.Until(ltime.Truncate(time.Hour).Add(time.Hour)))

		now := time.Now()

		reqDNS	:= atomic.SwapUint64(&stat.DNSRequest	, 0)
		reqHTTP	:= atomic.SwapUint64(&stat.HTTPReqeust	, 0)
		reqJson := atomic.SwapUint64(&stat.JsonRequest	, 0)

		stat.AddrLock.Lock()
		stat.AddrDNSLock.Lock()
		stat.AddrAPILock.Lock()

		reqUsers	:= len(stat.Addr)
		reqDnsUsers	:= len(stat.AddrDNS)
		reqApiUsers	:= len(stat.AddrAPI)

		stat.Save()

		stat.AddrLock.Unlock()
		stat.AddrDNSLock.Unlock()
		stat.AddrAPILock.Unlock()

		fmt.Fprintf(
			stat.fs,
			"[%s - %s] dns: %5d | cache: %5d | neg_cache: %5d || http: %5d | json : %5d | ip : %5d (%5d / %5d\n",
			ltime.Format("2006-01-02 15:04:05"),
			now.Format("2006-01-02 15:04:05"),
			dnsServer.cache.Length(),
			dnsServer.negCache.Length(),
			reqDNS,
			reqHTTP,
			reqJson,
			reqUsers,
			reqDnsUsers,
			reqApiUsers,
		)

		ltime = now
	}
}