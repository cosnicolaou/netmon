package main

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"sync"
	"time"
)

type ARPMonitor struct {
	l        *Logger
	interval time.Duration
	devices  map[string]Device
	mu       sync.Mutex
	previous []arpEntry
}

func NewARPMonitor(l *Logger, interval time.Duration) *ARPMonitor {
	return &ARPMonitor{
		l:        l,
		interval: interval,
		devices:  make(map[string]Device),
	}
}

func (m *ARPMonitor) log(ctx context.Context, format string, args ...any) {
	m.l.Log(ctx, "arp", format, args...)
}

func (m *ARPMonitor) warn(ctx context.Context, format string, args ...any) {
	m.l.Warn(ctx, "arp", format, args...)
}

func (m *ARPMonitor) MonitorAll(ctx context.Context, devs []Device) error {
	for _, dev := range devs {
		m.devices[dev.IP] = dev
	}
	for {
		table, err := readARPTable(ctx, m.devices)
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.interval):
		}
		added, removed, changed := compareTables(m.previous, table)
		shown := false
		for _, e := range added {
			m.log(ctx, "added arp entry", "name", m.devices[e.ip], "ip", e.ip, "mac", e.mac, "iface", e.iface)
			shown = true
		}
		for _, e := range removed {
			m.log(ctx, "removed arp entry", "name", m.devices[e.ip], "ip", e.ip, "mac", e.mac, "iface", e.iface)
			shown = true
		}
		for _, e := range changed {
			m.warn(ctx, "changed arp entry", "name", m.devices[e.current.ip], "ip", e.current.ip, "mac", e.current.mac, "iface", e.current.iface, "previous_mac", e.previous.mac, "previous_iface", e.previous.iface)
			shown = true
		}
		if !shown {
			m.log(ctx, "no changes in arp table")
		}
		m.mu.Lock()
		m.previous = table
		m.mu.Unlock()
	}
}

type changedARPEntry struct {
	previous, current arpEntry
}

func compareTables(a, b []arpEntry) (added, removed []arpEntry, changed []changedARPEntry) {
	am := make(map[string]arpEntry)
	bm := make(map[string]arpEntry)
	for _, e := range a {
		am[e.ip] = e
	}
	for _, e := range b {
		bm[e.ip] = e
	}
	for ip, e := range am {
		if _, ok := bm[ip]; !ok {
			removed = append(removed, e)
		}
	}
	for ip, e := range bm {
		if _, ok := am[ip]; !ok {
			added = append(added, e)
		}
	}
	for ip, be := range bm {
		ae, ok := am[ip]
		if !ok {
			continue
		}
		if be.mac != ae.mac {
			changed = append(changed, changedARPEntry{
				previous: ae,
				current:  be,
			})
		}
	}
	return added, removed, changed
}

type arpEntry struct {
	ip    string
	mac   string
	iface string
}

var arpRE = regexp.MustCompile(`\((?P<ip>[0-9.]+)\) at (?P<mac>[0-9a-f:]+) on (?P<iface>\w+)`)

func readARPTable(ctx context.Context, devices map[string]Device) (table []arpEntry, err error) {
	out, err := exec.CommandContext(ctx, "arp", "-an").Output()
	if err != nil {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		l := sc.Text()
		matches := arpRE.FindStringSubmatch(l)
		if len(matches) != 4 {
			continue
		}
		if _, ok := devices[matches[1]]; !ok {
			continue
		}
		table = append(table, arpEntry{
			ip:    matches[1],
			mac:   matches[2],
			iface: matches[3],
		})
	}
	return table, sc.Err()
}
