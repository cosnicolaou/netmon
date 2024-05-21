package main

import (
	"fmt"
	"net"
	"net/netip"
	"sync"
)

func ParseIPAddr(s string) (netip.Addr, error) {
	ip, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid IP address %q", s, err)
	}
	return ip, nil
}

var (
	outboundOnce sync.Once
	outboundV4   netip.Addr
	outboundV6   netip.Addr
)

func dialAddr(network, address string) (net.UDPAddr, error) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return net.UDPAddr{}, err
	}
	defer conn.Close()
	localAddr := *conn.LocalAddr().(*net.UDPAddr)
	localAddr.Port = 0
	return localAddr, nil
}

func PreferredOutboundAddrs() (v4, v6 netip.Addr, err error) {
	outboundOnce.Do(func() {
		var a net.UDPAddr
		a, err = dialAddr("udp4", "8.8.8.8:80")
		if err != nil {
			return
		}
		outboundV4, err = netip.ParseAddr(a.IP.String())
		a, err = dialAddr("udp6", "[2001:4860:4860::8888]:80")
		if err != nil {
			fmt.Printf("dialAddr: %v\n", err)
			err = nil
			return
		}
		outboundV6, _ = netip.ParseAddr(a.IP.String())
	})
	return outboundV4, outboundV6, err
}
