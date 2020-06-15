package src

import (
	"math"
	"math/rand"
	"net"
	"time"

	"github.com/getsentry/sentry-go"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

func ping(addr string, seq int, timeout time.Duration) (duration time.Duration, ok bool) {
	c, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		sentry.CaptureException(err)
		return 0, false
	}
	defer c.Close()

	dst, err := net.ResolveIPAddr("ip4", addr)
	if err != nil {
		sentry.CaptureException(err)
		return 0, false
	}

	nsec := time.Now().UnixNano()
	echoData := make([]byte, 8)
	for i := uint8(0); i < 8; i++ {
		echoData[i] = byte((nsec >> ((7 - i) * 8)) & 0xff)
	}

	ec := icmp.Echo{
		ID:   rand.Intn(math.MaxInt16),
		Seq:  seq,
		Data: echoData,
	}

	m := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &ec,
	}
	b, err := m.Marshal(nil)
	if err != nil {
		return 0, false
	}

	start := time.Now()
	n, err := c.WriteTo(b, dst)
	if err != nil || n != len(b) {
		if err != nil {
			sentry.CaptureException(err)
		}
		return 0, false
	}

	reply := make([]byte, 1500)
	err = c.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		sentry.CaptureException(err)
		return 0, false
	}

	n, _, err = c.ReadFrom(reply)
	if err != nil {
		sentry.CaptureException(err)
		return 0, false
	}
	duration = time.Since(start)

	rm, err := icmp.ParseMessage(1, reply[:n])
	if err != nil {
		sentry.CaptureException(err)
		return 0, false
	}

	if rm.Type != ipv4.ICMPTypeEchoReply {
		return 0, false
	}

	return duration, true
}
