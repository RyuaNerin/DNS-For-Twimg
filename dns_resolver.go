package main

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
	
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
)

type dnsResolver struct {
}

func newDNSResolver() *dnsResolver {
	return &dnsResolver {
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

func (h *dnsResolver) Resolve(host string) (ip net.IP, err error) {
	host = host + "."

	req := new(dns.Msg)
	req.SetQuestion(host, dns.TypeA)
	
	msg, err := h.Lookup("udp", req)
	if err != nil {
		return nil, err
	}

	for _, ans := range msg.Answer {
		switch ans.Header().Rrtype {
		case dns.TypeA:
			a := ans.(*dns.A)
			return a.A, nil
		}
	}

	return nil, ResolvError{host, "udp", nil}
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
			logrus.WithFields(logrus.Fields {
				"qname"			: qname,
				"nameserver"	: nameserver,
				"err"			: err.Error(),
				}).Debug("socket error")
			return
		}
		// If SERVFAIL happen, should return immediately and try another upstream resolver.
		// However, other Error code like NXDOMAIN is an clear response stating
		// that it has been verified no such domain existas and ask other resolvers
		// would make no sense. See more about #20
		if r != nil && r.Rcode != dns.RcodeSuccess {
			logrus.WithFields(logrus.Fields {
				"qname"			: qname,
				"nameserver"	: nameserver,
				}).Debug("failed to get an valid answer")

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

	nameservers := config.DNS.NameServer
	// Start lookup on each nameserver top-down, in every second
	for _, nameserver := range nameservers {
		wg.Add(1)
		go L(nameserver)
		// but exit early, if we have an answer
		select {
		case re := <-res:
			logrus.WithFields(logrus.Fields {
				"qname"			: qname,
				"nameserver"	: re.nameserver,
				"rtt"			: re.rtt,
				}).Debug("host resolved")

			return re.msg, nil
		case <-ticker.C:
			continue
		}
	}

	wg.Wait()
	select {
	case re := <-res:
		logrus.WithFields(logrus.Fields {
			"qname"			: qname,
			"nameserver"	: re.nameserver,
			"rtt"			: re.rtt,
			}).Debug("host resolved")

		return re.msg, nil
	default:
		return nil, ResolvError{qname, net, nameservers}
	}
}
