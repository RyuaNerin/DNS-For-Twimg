package main

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	notIPQuery = 0
	_IP4Query  = 4
	_IP6Query  = 6
)

type dnsHandler struct {
	dnsSfx			*suffixTreeNode
	cache, negCache MemoryCache
}

func newDNSHandler() (handler *dnsHandler) {
	handler = &dnsHandler {
		dnsSfx : newSuffixTreeRoot(),
	}

	handler.cache = MemoryCache {
		Backend		: make(map[string]Mesg),
		Expire		: config.DNS.ServerCacheExpire.Duration,
		Maxcount	: config.DNS.ServerCacheMaxCount,
	}

	handler.negCache = MemoryCache{
		Backend		: make(map[string]Mesg),
		Expire		: config.DNS.ServerCacheExpire.Duration,
		Maxcount	: config.DNS.ServerCacheMaxCount,
	}

	return
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

	mesg, err := h.Lookup(network, req)

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
		if q.Qtype != dns.TypeA {
			continue
		}

		dnsTwimgIPLock.RLock()
		ip, ok := dnsTwimgIP[q.Name]
		dnsTwimgIPLock.RUnlock()

		if ok {
			m := new(dns.Msg)
			m.SetReply(req)
			m.Authoritative = true

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

type ResolvError struct {
	qname, net  string
	nameservers []string
}
func (e ResolvError) Error() string {
	return fmt.Sprintf("%s resolv failed on %s (%s)", e.qname, strings.Join(e.nameservers, "; "), e.net)
}

// https://github.com/kenshinx/godns/blob/89cf763271800261c0ab38983a27c5b34f34f0a5/resolver.go#L131
func (h *dnsHandler) Lookup(net string, req *dns.Msg) (msg *dns.Msg, err error) {
	c := &dns.Client{
		Net				: net,
		ReadTimeout		: config.DNS.DNSLookupTimeout.Duration,
		WriteTimeout	: config.DNS.DNSLookupTimeout.Duration,
	}

	if net == "udp" {
		req = req.SetEdns0(65535, true)
	}

	qname := req.Question[0].Name

	type RResp struct {
		msg        *dns.Msg
		nameserver string
		rtt        time.Duration
	}
	res := make(chan *RResp, 1)
	var wg sync.WaitGroup
	L := func(nameserver string) {
		defer wg.Done()
		r, rtt, err := c.Exchange(req, nameserver)
		if err != nil {
			log.Printf("%s socket error on %s\n", qname, nameserver)
			log.Printf("error:%s\n", err.Error())
			return
		}
		// If SERVFAIL happen, should return immediately and try another upstream resolver.
		// However, other Error code like NXDOMAIN is an clear response stating
		// that it has been verified no such domain existas and ask other resolvers
		// would make no sense. See more about #20
		if r != nil && r.Rcode != dns.RcodeSuccess {
			log.Printf("%s failed to get an valid answer on %s\n", qname, nameserver)
			if r.Rcode == dns.RcodeServerFailure {
				return
			}
		}
		re := &RResp{r, nameserver, rtt}
		select {
		case res <- re:
		default:
		}
	}

	ticker := time.NewTicker(config.DNS.DNSLookupInterval.Duration)
	defer ticker.Stop()

	nameservers := h.getNameServer(qname)
	// Start lookup on each nameserver top-down, in every second
	for _, nameserver := range nameservers {
		wg.Add(1)
		go L(nameserver)
		// but exit early, if we have an answer
		select {
		case re := <-res:
			log.Printf("%s resolv on %s rtt: %v\n", qname, re.nameserver, re.rtt)
			return re.msg, nil
		case <-ticker.C:
			continue
		}
	}

	wg.Wait()
	select {
	case re := <-res:
		log.Printf("%s resolv on %s rtt: %v\n", qname, re.nameserver, re.rtt)
		return re.msg, nil
	default:
		return nil, ResolvError{qname, net, nameservers}
	}
}

func (h *dnsHandler) getNameServer(qname string) []string {
	queryKeys := strings.Split(qname, ".")
	queryKeys = queryKeys[:len(queryKeys)-1]

	ns := []string{}
	if v, ok := h.dnsSfx.search(queryKeys); ok {
		ns = append(ns, net.JoinHostPort(v, "53"))
		return ns
	}

	for _, nameserver := range config.DNS.NameServer {
		ns = append(ns, nameserver)
	}
	return ns
}