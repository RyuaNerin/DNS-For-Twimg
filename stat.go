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

	AddressesLock	sync.Mutex
	Addresses		map[uint32]struct{}
}

var stat = Stat {
	Addresses : make(map[uint32]struct{}),
}

func (stat *Stat) Load() {
	fs, err := os.Open(config.Path.StatSave)
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

		stat.Addresses[uint32(buff[0]) << 24 | uint32(buff[1]) << 16 | uint32(buff[2]) << 8 | uint32(buff[3])] = struct{}{}
	}
}
func (stat *Stat) Save() {
	fs, err := os.OpenFile(config.Path.StatSave, os.O_CREATE | os.O_WRONLY, 644)
	if err != nil {
		logRusPanic.Error(err)
		return
	}
	defer fs.Close()

	fs.Truncate(0)
	fs.Seek(0, 0)

	buff := make([]byte, 4)
	for u := range stat.Addresses {
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

	stat.AddressesLock.Lock()
	stat.Addresses[u] = struct{}{}
	stat.AddressesLock.Unlock()
}

func (stat *Stat) AddHTTPReqeust() {
	atomic.AddUint64(&stat.HTTPReqeust, 1)
}

func (stat *Stat) AddJsonReqeust() {
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

		stat.AddressesLock.Lock()
		reqIP	:= len(stat.Addresses)
		stat.AddressesLock.Unlock()

		stat.Save()

		fmt.Fprintf(
			stat.fs,
			"[%s - %s] dns: %10d | cache: %10d | neg_cache: %10d | http: %10d | json : %10d | ip : %10d\n",
			ltime.Format("2006-01-02 15:04:05"),
			now.Format("2006-01-02 15:04:05"),
			dnsServer.cache.Length(),
			dnsServer.negCache.Length(),
			reqDNS,
			reqHTTP,
			reqJson,
			reqIP,
		)

		ltime = now
	}
}