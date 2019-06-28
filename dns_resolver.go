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

type dnsResolver struct {
	dnsSfx *suffixTreeNode
}

func newDNSResolver() *dnsResolver {
	return &dnsResolver {
		dnsSfx : newSuffixTreeRoot(),
	}
}

var defaultDNSResolver = newDNSResolver()

type ResolvError struct {
	qname, net  string
	nameservers []string
}
func (e ResolvError) Error() string {
	return fmt.Sprintf("%s resolv failed on %s (%s)", e.qname, strings.Join(e.nameservers, "; "), e.net)
}

// https://github.com/kenshinx/godns/blob/89cf763271800261c0ab38983a27c5b34f34f0a5/resolver.go#L131
func (h *dnsResolver) Lookup(net string, req *dns.Msg) (msg *dns.Msg, err error) {
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

func (h *dnsResolver) getNameServer(qname string) []string {
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