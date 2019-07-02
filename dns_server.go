package main

import (
	"net"
	"sync"
	
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
)

type DNSServer struct {
	cache, negCache MemoryCache

	dnsTCPMux	dns.ServeMux
	dnsUDPMux	dns.ServeMux

	dnsTCP		dns.Server
	dnsUDP		dns.Server

	cdnLock		sync.Mutex
	cdnHostQ	[]Question
}

var dnsServer = DNSServer {
	cache : MemoryCache {
		Backend		: make(map[string]Mesg),
		Expire		: config.DNS.ServerCacheExpire.Duration,
		Maxcount	: config.DNS.ServerCacheMaxCount,
	},
	negCache : MemoryCache{
		Backend		: make(map[string]Mesg),
		Expire		: config.DNS.ServerCacheExpire.Duration,
		Maxcount	: config.DNS.ServerCacheMaxCount,
	},
}

func (sv *DNSServer) SetCDN(c CdnStatusCollection) {
	sv.cdnLock.Lock()
	defer sv.cdnLock.Unlock()

	for _, key := range sv.cdnHostQ {
		sv.cache.Remove(key)
	}
	sv.cdnHostQ = sv.cdnHostQ[:0]

	for host, result := range c {
		if len(result) == 0 {
			continue
		}

		host = host + "."

		aRecord := &dns.A {
			A	: result[0].IP,
			Hdr	: dns.RR_Header {
				Name	: host,
				Rrtype	: dns.TypeA,
				Class	: dns.ClassINET,
				Ttl		: uint32(config.Test.RefreshInterval.Duration.Seconds()),
			},
		}

		sv.addMsg(host, dns.TypeA		, aRecord)
		sv.addMsg(host, dns.TypeAAAA	, nil)
		sv.addMsg(host, dns.TypeCNAME	, nil)
	}
}

func (sv *DNSServer) addMsg(host string, qType uint16, rr dns.RR) {
	q := newQuestion(host, qType, dns.ClassINET)
	m := &dns.Msg {
		Question : []dns.Question {
			dns.Question {
				Name	: host,
				Qclass	: dns.ClassINET,
				Qtype	: qType,
			},
		},
	}

	if rr != nil {
		m.Answer = append(m.Answer, rr)
	}
	
	sv.cdnHostQ = append(sv.cdnHostQ, q)
	sv.cache.Set(q, m, true)
}

func (sv *DNSServer) Start() {
	sv.dnsTCP = dns.Server {
		Net				: "tcp",
		Handler			: dns.HandlerFunc(sv.HandleTCP),
		ReadTimeout		: config.DNS.DNSLookupTimeout.Duration,
		WriteTimeout	: config.DNS.DNSLookupTimeout.Duration,
	}

	sv.dnsUDP = dns.Server {
		Net				: "udp",
		Handler			: dns.HandlerFunc(sv.handleUDP),
		UDPSize			: 65535,
		ReadTimeout		: config.DNS.DNSLookupTimeout.Duration,
		WriteTimeout	: config.DNS.DNSLookupTimeout.Duration,
	}

	go sv.listenAndServe(&sv.dnsTCP)
	go sv.listenAndServe(&sv.dnsUDP)
}

func (sv *DNSServer) listenAndServe(ds *dns.Server) {
	err := ds.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

func (sv *DNSServer) Restart() {
	err := sv.dnsTCP.Shutdown()
	if err != nil {
		panic(err)
	}
	
	err = sv.dnsUDP.Shutdown()
	if err != nil {
		panic(err)
	}

	sv.Start()
}

func (sv *DNSServer) HandleTCP(w dns.ResponseWriter, req *dns.Msg) {
	sv.handle("tcp", w, req)
}

func (sv *DNSServer) handleUDP(w dns.ResponseWriter, req *dns.Msg) {
	sv.handle("udp", w, req)
}

func (sv *DNSServer) handle(network string, w dns.ResponseWriter, req *dns.Msg) {
	q := req.Question[0]
	Q := newQuestion(q.Name, q.Qtype, q.Qclass)

	var remote net.IP
	if network == "tcp" {
		remote = w.RemoteAddr().(*net.TCPAddr).IP
	} else {
		remote = w.RemoteAddr().(*net.UDPAddr).IP
	}
	stat.AddDNSReqeust(remote)

	logrus.WithFields(logrus.Fields {
		"remote"	: remote,
		"query"		: Q.String(),
		}).Debug("lookup")

	isIPQuery := sv.isIPQuery(q)

	// Only query cache when qtype == 'A'|'AAAA' , qclass == 'IN'
	if isIPQuery {
		mesg, err := sv.cache.Get(Q)
		if err != nil {
			if mesg, err = sv.negCache.Get(Q); err != nil {
				logrus.WithFields(logrus.Fields {
					"query"		: Q.String(),
					}).Debug("no cache")
			} else {
				logrus.WithFields(logrus.Fields {
					"query"		: Q.String(),
					}).Debug("negative cache")

				dns.HandleFailed(w, req)
				return
			}
		} else {
			logrus.WithFields(logrus.Fields {
				"query"		: Q.String(),
				}).Debug("hit cache")

			// we need this copy against concurrent modification of Id
			msg := *mesg
			msg.Id = req.Id
			w.WriteMsg(&msg)
			return
		}
	}

	mesg, err := defaultDNSResolver.Lookup(network, req)

	if err != nil {
		logrus.WithFields(logrus.Fields {
			"error"	: err.Error(),
			}).Debug("Resolve query error")

		dns.HandleFailed(w, req)

		// cache the failure, too!
		if err = sv.negCache.Set(Q, nil, false); err != nil {
			logrus.WithFields(logrus.Fields {
				"query" : Q.String(),
				"error"	: err.Error(),
				}).Debug("set negative cache failed")
		}
		return
	}

	w.WriteMsg(mesg)

	if isIPQuery && len(mesg.Answer) > 0 {
		err = sv.cache.Set(Q, mesg, false)
		if err != nil {
			logrus.WithFields(logrus.Fields {
				"query" : Q.String(),
				"error"	: err.Error(),
				}).Debug("set cache failed")
		} else {
			logrus.WithFields(logrus.Fields {
				"query" : Q.String(),
				}).Debug("set cache")
		}
	}
}

func (sv *DNSServer) isIPQuery(q dns.Question) bool {
	return q.Qclass == dns.ClassINET && (q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA)
}