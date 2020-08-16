package tester

import (
	"net"
	"strings"
	"sync"
	"time"

	"twimgdns/src/common/cfg"

	"github.com/getsentry/sentry-go"
	"github.com/miekg/dns"
)

func resolve(dnsAddr []string, host string) (ip net.IP, ok bool) {
	dnsClient := dns.Client{
		Net:          "udp",
		Timeout:      cfg.V.DNS.Client.Timeout.Timeout,
		ReadTimeout:  cfg.V.DNS.Client.Timeout.ReadTimeout,
		WriteTimeout: cfg.V.DNS.Client.Timeout.WriteTimeout,
		DialTimeout:  cfg.V.DNS.Client.Timeout.DialTimeout,
	}

	if !strings.HasSuffix(host, ".") {
		host = host + "."
	}

	var msg dns.Msg
	msg.SetQuestion(host, dns.TypeA)
	msg.SetEdns0(4096, true)

	rt := func(r *dns.Msg) (ip net.IP, ok bool) {
		if r == nil || r.Rcode != dns.RcodeSuccess {
			return nil, false
		}

		for _, ans := range r.Answer {
			if ans.Header().Rrtype == dns.TypeA {
				return ans.(*dns.A).A, true
			}
		}

		return nil, false
	}

	for i, addr := range dnsAddr {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			dnsAddr[i] = addr + ":53"
		}
	}

	if len(dnsAddr) == 1 {
		r, _, err := dnsClient.Exchange(&msg, dnsAddr[0])
		if err != nil {
			sentry.CaptureException(err)
			return nil, false
		}
		return rt(r)
	}

	var wg sync.WaitGroup
	res := make(chan *dns.Msg, 1)

	ticker := time.NewTicker(cfg.V.DNS.Client.LookupInterval)
	defer ticker.Stop()

	for _, addr := range dnsAddr {
		wg.Add(1)

		go func(nameserver string) {
			defer wg.Done()
			r, _, err := dnsClient.Exchange(&msg, nameserver)
			if err != nil {
				sentry.CaptureException(err)
				return
			}
			if r != nil && r.Rcode != dns.RcodeSuccess {
				return
			}

			select {
			case res <- r:
			default:
			}
		}(addr)

		select {
		case r := <-res:
			return rt(r)
		case <-ticker.C:
			continue
		}
	}

	wg.Wait()

	select {
	case r := <-res:
		return rt(r)
	default:
	}

	return nil, false
}
