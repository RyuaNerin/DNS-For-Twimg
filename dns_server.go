package main

import (
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	dnsTwimgRecordTTL	= 30 * 60 // 30 분
	dnsTimeOut			= 5 * time.Second
)

var (
	dnsTwimgIPLock	sync.RWMutex
	dnsTwimgIP		= make(map[string]net.IP)

	dnsTCP dns.Server
	dnsUDP dns.Server
)

func setDNSHostIP(c CdnStatusCollection) {
	dnsTwimgIPLock.Lock()
	for host := range dnsTwimgIP {
		delete(dnsTwimgIP, host)
	}
	for host, result := range c {
		dnsTwimgIP[host + "."] = result[0].IP
	}
	dnsTwimgIPLock.Unlock()
}

func startDNSServer() {
	handler := newDNSHandler()

	tcpHandler := dns.NewServeMux()
	tcpHandler.HandleFunc(".", handler.HandleTCP)

	udpHandler := dns.NewServeMux()
	udpHandler.HandleFunc(".", handler.handleUDP)

	dnsTCP = dns.Server{
		Net : "tcp",
		Handler : tcpHandler,
		ReadTimeout : dnsTimeOut,
		WriteTimeout : dnsTimeOut,
	}

	dnsUDP = dns.Server {
		Net : "udp",
		Handler : udpHandler,
		UDPSize : 65535,
		ReadTimeout : dnsTimeOut,
		WriteTimeout : dnsTimeOut,
	}

	go listenAndServe(&dnsTCP)
	go listenAndServe(&dnsUDP)
}

func listenAndServe(ds *dns.Server) {
	err := ds.ListenAndServe()
	if err != nil {
		panic(err)
	}
}
