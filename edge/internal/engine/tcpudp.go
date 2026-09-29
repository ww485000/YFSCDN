package engine

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"sync"

	"edgecdn/edge/internal/contract"
)

// rawPlan describes one raw (tcp/udp) listener from a site spec.
type rawPlan struct {
	proto   string // tcp | udp
	listen  string
	siteID  int64
	origins []contract.Origin
	sched   contract.Scheduling
}

// rawProxy forwards one raw listener to its origin group.
type rawProxy struct {
	proto   string
	addr    string
	siteID  int64
	origins []contract.Origin
	sched   contract.Scheduling

	mu       sync.RWMutex
	listener net.Listener
	udpConn  net.PacketConn
	wg       sync.WaitGroup
	closed   bool
}

func newRawProxy(proto, listen string, siteID int64, origins []contract.Origin, sched contract.Scheduling) (*rawProxy, error) {
	if len(origins) == 0 {
		return nil, fmt.Errorf("no origins for %s listener %s", proto, listen)
	}
	return &rawProxy{proto: proto, addr: listen, siteID: siteID, origins: origins, sched: sched}, nil
}

func (rp *rawProxy) start() error {
	switch rp.proto {
	case "tcp":
		c, err := net.Listen("tcp", rp.addr)
		if err != nil {
			return err
		}
		rp.listener = c
		go rp.tcpLoop()
	case "udp":
		c, err := net.ListenPacket("udp", rp.addr)
		if err != nil {
			return err
		}
		rp.udpConn = c
		go rp.udpLoop()
	}
	return nil
}

func (rp *rawProxy) pickOrigin() string {
	// resolve origins to addresses with default ports
	var addrs []string
	for i := range rp.origins {
		o := &rp.origins[i]
		if o.Status == "off" {
			continue
		}
		a := o.Addr
		if !containsColon(a) {
			p := 80
			if o.SSL {
				p = 443
			}
			a = a + ":" + itoa(p)
		}
		addrs = append(addrs, a)
	}
	if len(addrs) == 0 {
		return ""
	}
	switch rp.sched.Type {
	case "round_robin":
		return addrs[0]
	default:
		return addrs[rand.Intn(len(addrs))]
	}
}

func containsColon(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return true
		}
	}
	return false
}

func (rp *rawProxy) tcpLoop() {
	for {
		client, err := rp.listener.Accept()
		if err != nil {
			return
		}
		rp.wg.Add(1)
		go func(c net.Conn) {
			defer rp.wg.Done()
			defer c.Close()
			dst, err := net.Dial("tcp", rp.pickOrigin())
			if err != nil {
				log.Printf("[engine] tcp proxy dial %s: %v", rp.addr, err)
				return
			}
			defer dst.Close()
			done := make(chan struct{}, 2)
			go func() {
				_, _ = io.Copy(dst, c)
				if tc, ok := dst.(*net.TCPConn); ok {
					_ = tc.CloseWrite()
				}
				done <- struct{}{}
			}()
			go func() {
				_, _ = io.Copy(c, dst)
				if tc, ok := c.(*net.TCPConn); ok {
					_ = tc.CloseWrite()
				}
				done <- struct{}{}
			}()
			<-done
		}(client)
	}
}

func (rp *rawProxy) udpLoop() {
	buf := make([]byte, 64*1024)
	for {
		n, client, err := rp.udpConn.ReadFrom(buf)
		if err != nil {
			return
		}
		dst := rp.pickOrigin()
		if dst == "" {
			continue
		}
		cp, err := net.ResolveUDPAddr("udp", dst)
		if err != nil {
			continue
		}
		conn, err := net.DialUDP("udp", nil, cp)
		if err != nil {
			continue
		}
		_, _ = conn.Write(buf[:n])
		resp := make([]byte, 64*1024)
		m, _, err := conn.ReadFrom(resp)
		if err == nil {
			_, _ = rp.udpConn.WriteTo(resp[:m], client)
		}
		conn.Close()
	}
}

// update re-points the proxy to a new origin set (in place).
func (rp *rawProxy) update(origins []contract.Origin, sched contract.Scheduling) {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	rp.origins = origins
	rp.sched = sched
	log.Printf("[engine] %s proxy %s re-pointed", rp.proto, rp.addr)
}

func (rp *rawProxy) close() {
	rp.mu.Lock()
	if rp.closed {
		rp.mu.Unlock()
		return
	}
	rp.closed = true
	rp.mu.Unlock()
	var err error
	if rp.listener != nil {
		err = rp.listener.Close()
	}
	if rp.udpConn != nil {
		err2 := rp.udpConn.Close()
		if err == nil {
			err = err2
		}
	}
	if err != nil {
		log.Printf("[engine] close %s listener: %v", rp.proto, err)
	}
	rp.wg.Wait()
}

// ErrClosed is returned when the proxy listener is gone.
var ErrClosed = errors.New("listener closed")
