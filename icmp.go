package main

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"cloudeng.io/sync/errgroup"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type ICMPMonitor struct {
	l     *Logger
	icmp4 *icmp.PacketConn
	icmp6 *icmp.PacketConn
	rx4   *icmpConn
	rx6   *icmpConn
}

func NewICMPMonitor(l *Logger) *ICMPMonitor {
	return &ICMPMonitor{l: l}
}

func (m *ICMPMonitor) log(ctx context.Context, format string, args ...any) {
	m.l.Log(ctx, "ping", format, args...)
}

func (m *ICMPMonitor) warn(ctx context.Context, format string, args ...any) {
	m.l.Warn(ctx, "ping", format, args...)
}

type icmpEcho struct {
	reply *icmp.Echo
	peer  net.Addr
}

type icmpConn struct {
	sync.Mutex
	id       int
	conn     *icmp.PacketConn
	echoType icmp.Type
	waiters  map[int]chan icmpEcho
}

func newICMPConn(conn *icmp.PacketConn, echoType icmp.Type) *icmpConn {
	return &icmpConn{
		conn:     conn,
		echoType: echoType,
		waiters:  map[int]chan icmpEcho{},
	}
}

func (c *icmpConn) register(ch chan icmpEcho) int {
	c.Lock()
	defer c.Unlock()
	c.id++
	c.waiters[c.id] = ch
	return c.id
}

func (c *icmpConn) deregister(id int) {
	c.Lock()
	defer c.Unlock()
	delete(c.waiters, id)
}

func (c *icmpConn) forwardEcho(ctx context.Context, peer net.Addr, m *icmp.Echo) error {
	c.Lock()
	defer c.Unlock()
	ch, ok := c.waiters[m.ID]
	if ok {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- icmpEcho{reply: m, peer: peer}:
		}
		return nil
	}
	return fmt.Errorf("no listener for id: %v", m.ID)
}

func (c *icmpConn) listenLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rb := make([]byte, 1500)
		n, peer, err := c.conn.ReadFrom(rb)
		if err != nil {
			return err
		}
		rm, err := icmp.ParseMessage(c.echoType.Protocol(), rb[:n])
		if err != nil {
			return err
		}
		switch rm.Type {
		case c.echoType:
			err = c.forwardEcho(ctx, peer, rm.Body.(*icmp.Echo))
		default:
			err = fmt.Errorf("unexpected message type: %v", rm.Type)
		}
	}
}

func (m *ICMPMonitor) createListeners() error {
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return err
	}
	m.icmp4 = conn
	m.rx4 = newICMPConn(conn, ipv4.ICMPTypeEchoReply)
	conn, err = icmp.ListenPacket("udp6", "::")
	if err != nil {
		return err
	}
	m.icmp6 = conn
	m.rx6 = newICMPConn(conn, ipv6.ICMPTypeEchoReply)
	return nil
}

func (m *ICMPMonitor) MonitorAll(ctx context.Context, devs []ICMPDevice) error {
	if err := m.createListeners(); err != nil {
		return err
	}
	var g errgroup.T
	g.Go(func() error {
		return m.rx4.listenLoop(ctx)
	})
	g.Go(func() error {
		return m.rx6.listenLoop(ctx)
	})
	for _, dev := range devs {
		g.Go(func() error {
			return m.MonitorDevice(ctx, dev)
		})
	}
	select {
	case <-ctx.Done():
		m.icmp4.Close()
		m.icmp6.Close()
		break
	}
	return g.Wait()
}

func (m *ICMPMonitor) MonitorDevice(ctx context.Context, dev ICMPDevice) error {
	var echoType icmp.Type
	var id int
	conn := m.icmp4
	echoType = ipv4.ICMPTypeEcho
	ch := make(chan icmpEcho, 1)
	if dev.ipAddr.Is6() {
		echoType = ipv6.ICMPTypeEchoRequest
		conn = m.icmp6
		id = m.rx6.register(ch)
	} else {
		id = m.rx4.register(ch)
	}
	dst := &net.UDPAddr{IP: dev.ipAddr.AsSlice()}
	seq := 0
	for {
		err := m.ping(ctx, dev, dst, echoType, id, seq, conn, ch, dev.Timeout)
		if err != nil {
			m.warn(ctx, "failed", "name", dev.Name, "dst", dst.IP, "error", err.Error())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(dev.Period):
			seq++
		}
	}
}

func (m *ICMPMonitor) ping(ctx context.Context, dev ICMPDevice, dst *net.UDPAddr, echoType icmp.Type, id, seq int, conn *icmp.PacketConn, ch chan icmpEcho, timeout time.Duration) error {
	wm := icmp.Message{
		Type: echoType,
		Code: 0,
		Body: &icmp.Echo{
			ID:   id,
			Seq:  seq,
			Data: []byte("HELLO-R-U-THERE"),
		},
	}
	wb, err := wm.Marshal(nil)
	if err != nil {
		return err
	}
	start := time.Now()
	if _, err := conn.WriteTo(wb, dst); err != nil {
		return err
	}
	select {
	case <-time.After(timeout):
		m.warn(ctx, "timeout", "name", dev.Name, "dst", dst.IP, "id", id, "seq", seq, "timeout", timeout.String(), "took", time.Since(start).String())
		break
	case <-ctx.Done():
		return err
	case msg := <-ch:
		m.log(ctx, "ok", "name", dev.Name, "peer", msg.peer, "id", msg.reply.ID, "seq", msg.reply.Seq, "took", time.Since(start).String())
	}
	return err
}
