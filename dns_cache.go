// https://github.com/kenshinx/godns/blob/89cf763271800261c0ab38983a27c5b34f34f0a5/cache.go

package main

import (
	"crypto/md5"
	"encoding/hex"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type KeyNotFound struct {
	key string
}
func (e KeyNotFound) Error() string {
	return e.key + " " + "not found"
}

type KeyExpired struct {
	Key string
}
func (e KeyExpired) Error() string {
	return e.Key + " " + "expired"
}

type CacheIsFull struct {
}
func (e CacheIsFull) Error() string {
	return "Cache is Full"
}

type MemoryCache struct {
	Backend  map[string]Mesg
	Expire   time.Duration
	Maxcount int
	mu       sync.RWMutex
}

type Mesg struct {
	Msg		*dns.Msg
	Expire	time.Time
}

type Question struct {
	qname  	string
	qtype  	string
	qclass	string
	
	hash	string
}
func (q *Question) String() string {
	return q.qname + " " + q.qclass + " " + q.qtype
}

func newQuestion(qname string, qtype uint16, qclass uint16) (q Question) {
	q.qname = qname
	q.qtype = dns.TypeToString[qtype]
	q.qclass = dns.ClassToString[qclass]

	h := md5.New()
	h.Write(s2b(q.String()))
	q.hash = hex.EncodeToString(h.Sum(nil))

	return
}

func (c *MemoryCache) Get(key string) (*dns.Msg, error) {
	c.mu.RLock()
	mesg, ok := c.Backend[key]
	c.mu.RUnlock()
	if !ok {
		return nil, KeyNotFound{key}
	}

	if mesg.Expire.Before(time.Now()) {
		c.Remove(key)
		return nil, KeyExpired{key}
	}

	return mesg.Msg, nil
}

func (c *MemoryCache) Set(key string, msg *dns.Msg) error {
	if c.Full() && !c.Exists(key) {
		return CacheIsFull{}
	}

	expire := time.Now().Add(c.Expire)
	mesg := Mesg{msg, expire}
	c.mu.Lock()
	c.Backend[key] = mesg
	c.mu.Unlock()
	return nil
}

func (c *MemoryCache) Remove(key string) error {
	c.mu.Lock()
	delete(c.Backend, key)
	c.mu.Unlock()
	return nil
}

func (c *MemoryCache) Exists(key string) bool {
	c.mu.RLock()
	_, ok := c.Backend[key]
	c.mu.RUnlock()
	return ok
}

func (c *MemoryCache) Length() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.Backend)
}

func (c *MemoryCache) Full() bool {
	// if Maxcount is zero. the cache will never be full.
	if c.Maxcount == 0 {
		return false
	}
	return c.Length() >= c.Maxcount
}