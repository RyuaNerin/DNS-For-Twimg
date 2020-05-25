package src

import (
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

func resolve(c *dns.Client, dnsAddr []string, host string) (ip net.IP, err error) {
	if !strings.HasSuffix(host, ".") {
		host = host + "."
	}

	var msg dns.Msg
	msg.SetQuestion(host, dns.TypeA)
	msg.SetEdns0(4096, true)

	rt := func(r *dns.Msg) (ip net.IP, err error) {
		if r == nil || r.Rcode != dns.RcodeSuccess {
			return nil, errors.New("r.Rcode is not dns.RcodeSuccess")
		}

		for _, ans := range r.Answer {
			if ans.Header().Rrtype == dns.TypeA {
				return ans.(*dns.A).A, nil
			}
		}

		return nil, errors.New("r.Answer did not contains dns.TypeA")
	}

	for i, addr := range dnsAddr {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			dnsAddr[i] = addr + ":53"
		}
	}

	if len(dnsAddr) == 1 {
		r, _, err := c.Exchange(&msg, dnsAddr[0])
		if err != nil {
			return nil, err
		}
		return rt(r)
	}

	var wg sync.WaitGroup
	res := make(chan *dns.Msg, 1)

	ticker := time.NewTicker(config.DNS.Client.LookupInterval)
	defer ticker.Stop()

	for _, addr := range dnsAddr {
		wg.Add(1)

		go func(nameserver string) {
			defer wg.Done()
			r, _, err := c.Exchange(&msg, nameserver)
			if err != nil {
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

	return nil, errors.New("failed to get an valid answer")
}
