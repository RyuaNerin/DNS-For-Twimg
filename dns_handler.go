package main

import (
	"log"
	"net"

	"github.com/miekg/dns"
)

const (
	notIPQuery = 0
	_IP4Query  = 4
	_IP6Query  = 6
)

type dnsHandler struct {
	cache, negCache MemoryCache
}

func newDNSHandler() (handler *dnsHandler) {
	return &dnsHandler {
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
}

func (h *dnsHandler) HandleTCP(w dns.ResponseWriter, req *dns.Msg) {
	h.handle("tcp", w, req)
}

func (h *dnsHandler) handleUDP(w dns.ResponseWriter, req *dns.Msg) {
	h.handle("udp", w, req)
}

func (h *dnsHandler) handle(network string, w dns.ResponseWriter, req *dns.Msg) {
	if h.handleTwimg(w, req) {
		return
	}

	q := req.Question[0]
	Q := newQuestion(q.Name, q.Qtype, q.Qclass)

	var remote net.IP
	if network == "tcp" {
		remote = w.RemoteAddr().(*net.TCPAddr).IP
	} else {
		remote = w.RemoteAddr().(*net.UDPAddr).IP
	}
	log.Printf("%s lookup　%s\n", remote, Q.String())

	IPQuery := h.isIPQuery(q)

	// Only query cache when qtype == 'A'|'AAAA' , qclass == 'IN'
	if IPQuery > 0 {
		mesg, err := h.cache.Get(Q.hash)
		if err != nil {
			if mesg, err = h.negCache.Get(Q.hash); err != nil {
				log.Printf("%s didn't hit cache\n", Q.String())
			} else {
				log.Printf("%s hit negative cache\n", Q.String())
				dns.HandleFailed(w, req)
				return
			}
		} else {
			log.Printf("%s hit cache\n", Q.String())
			// we need this copy against concurrent modification of Id
			msg := *mesg
			msg.Id = req.Id
			w.WriteMsg(&msg)
			return
		}
	}

	mesg, err := defaultDNSResolver.Lookup(network, req)

	if err != nil {
		log.Printf("Resolve query error %s\n", err)
		dns.HandleFailed(w, req)

		// cache the failure, too!
		if err = h.negCache.Set(Q.hash, nil); err != nil {
			log.Printf("Set %s negative cache failed: %v", Q.String(), err)
		}
		return
	}

	w.WriteMsg(mesg)

	if IPQuery > 0 && len(mesg.Answer) > 0 {
		err = h.cache.Set(Q.hash, mesg)
		if err != nil {
			log.Printf("Set %s cache failed: %s", Q.String(), err.Error())
		}
		log.Printf("Insert %s into cache", Q.String())
	}
}

func (h *dnsHandler) handleTwimg(w dns.ResponseWriter, req *dns.Msg) (pass bool) {
	if req.Opcode != dns.OpcodeQuery {
		return
	}

	for _, q := range req.Question {
		dnsTwimgIPLock.RLock()
		ip, ok := dnsTwimgIP[q.Name]
		dnsTwimgIPLock.RUnlock()

		if ok {
			m := new(dns.Msg)
			m.SetReply(req)
			m.Authoritative = true

			
			switch q.Qtype {
			case dns.TypeA:
				aRecord := &dns.A {
					A	: ip,
					Hdr	: dns.RR_Header {
						Name	: q.Name,
						Rrtype	: dns.TypeA,
						Class	: dns.ClassINET,
						Ttl		: dnsTwimgRecordTTL,
					},

				}
				m.Answer = append(m.Answer, aRecord)
				w.WriteMsg(m)
				return true

			case dns.TypeAAAA:
				w.WriteMsg(m)
				return true

			case dns.TypeCNAME:
				w.WriteMsg(m)
				return true
			}
		}
	}

	return
}

func (h *dnsHandler) isIPQuery(q dns.Question) int {
	if q.Qclass != dns.ClassINET {
		return notIPQuery
	}

	switch q.Qtype {
	case dns.TypeA:
		return _IP4Query
	case dns.TypeAAAA:
		return _IP6Query
	default:
		return notIPQuery
	}
}