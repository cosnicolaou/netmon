package main

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type RouteMonitor struct {
	l        *Logger
	period   time.Duration
	devices  map[string]Device
	mu       sync.Mutex
	previous []routeEntry
}

func NewRouteMonitor(l *Logger, period time.Duration) *RouteMonitor {
	return &RouteMonitor{
		l:       l,
		period:  period,
		devices: make(map[string]Device),
	}
}

func (m *RouteMonitor) log(ctx context.Context, format string, args ...any) {
	m.l.Log(ctx, "route", format, args...)
}

func (m *RouteMonitor) warn(ctx context.Context, format string, args ...any) {
	m.l.Warn(ctx, "route", format, args...)
}

func (m *RouteMonitor) MonitorAll(ctx context.Context, devs []Device) error {
	for _, dev := range devs {
		m.devices[dev.IP] = dev
	}
	for {
		table, err := readRoutingTable(ctx, m.devices)
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.period):
		}
		for _, e := range table {
			if e.exp > 0 && e.exp < time.Second*30 {
				m.log(ctx, "route expiring soon", "dst", e.dst, "gw", e.gw, "flags", e.flags, "iface", e.iface, "exp", e.exp.String())
			}
		}
		added, removed, changed := compareRoutingTables(m.previous, table)
		for _, e := range added {
			m.log(ctx, "added route table entry", "dst", e.dst, "gw", e.gw, "flags", e.flags, "iface", e.iface, "exp", e.exp.String())
		}
		for _, e := range removed {
			m.log(ctx, "removed route table entry", "dst", e.dst, "gw", e.gw, "flags", e.flags, "iface", e.iface, "exp", e.exp.String())
		}
		for _, e := range changed {
			m.warn(ctx, "changed route table entry", "dst", e.current.dst, "gw", e.current.gw, "flags", e.current.flags, "iface", "exp", e.current.exp.String(), e.current.iface, "previous_gw", e.previous.gw, "previous_flags", e.previous.flags, "previous_iface", e.previous.iface, "exp", e.previous.exp.String())
		}
		m.mu.Lock()
		m.previous = table
		m.mu.Unlock()
	}
}

type changedRouteEntry struct {
	previous, current routeEntry
}

func compareRoutingTables(previous, current []routeEntry) (added, removed []routeEntry, changed []changedRouteEntry) {
	previousMap := make(map[string]routeEntry)
	for _, e := range previous {
		previousMap[e.dst] = e
	}

	currentMap := make(map[string]routeEntry)
	for _, e := range current {
		currentMap[e.dst] = e
	}

	for dst, e := range previousMap {
		if _, ok := currentMap[dst]; !ok {
			removed = append(removed, e)
		}
	}

	for dst, e := range currentMap {
		prev, ok := previousMap[dst]
		if !ok {
			added = append(added, e)
			continue
		}
		if prev.dst != e.dst || prev.gw != e.gw || prev.flags != e.flags || prev.iface != e.iface {
			changed = append(changed, changedRouteEntry{
				previous: prev,
				current:  e,
			})
		}
	}
	return
}

type routeEntry struct {
	dst   string
	gw    string
	flags string
	iface string
	exp   time.Duration
}

var netstatRE = regexp.MustCompile(`(?P<dest>[0-9.]+|default)\s+(?P<gw>[0-9a-f:]+)\s+(?P<flags>\w+)\s+(?P<if>\w+)\s+(?P<exp>\d+)*`)

func readRoutingTable(ctx context.Context, devices map[string]Device) (table []routeEntry, err error) {
	out, err := exec.CommandContext(ctx, "netstat", "-rn").Output()
	if err != nil {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		l := sc.Text()
		if len(l) == 0 || strings.HasPrefix(l, "Internet") || strings.HasPrefix(l, "Destination") {
			continue
		}
		matches := netstatRE.FindStringSubmatch(l)
		var entry routeEntry
		switch len(matches) {
		case 6:
			if len(matches[5]) > 0 {
				exp, _ := time.ParseDuration(matches[5])
				entry.exp = exp
			}
			fallthrough
		case 5:
			entry.dst = matches[1]
			entry.gw = matches[2]
			entry.flags = matches[3]
			entry.iface = matches[4]
		default:
			continue
		}
		if _, ok := devices[matches[1]]; !ok {
			continue
		}
		table = append(table, entry)
	}
	return table, sc.Err()
}
