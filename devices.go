package main

import (
	"context"
	"fmt"

	"cloudeng.io/sync/errgroup"
)

type DeviceMonitorFlags struct {
	ConfigFlags
	LogFile string `subcmd:"log-file,netmon.slog,the file to write structured logs to"`
	Ping    bool   `subcmd:"ping,false,enable pinging of devices"`
	ARP     bool   `subcmd:"arp,false,enable arp monitoring"`
	RTSP    bool   `subcmd:"rtsp,false,enable rtsp monitoring"`
	Routing bool   `subcmd:"routing,false,enable routing monitoring"`
	Syslog  bool   `subcmd:"syslog,false,enable syslog server"`
	CGI     bool   `subcmd:"cgi,false,enable cgi invocations"`
	DryRun  bool   `subcmd:"dry-run,false,show only configuration information"`
}

type Devices struct {
}

func (d *Devices) Monitor(ctx context.Context, flags any, args []string) error {
	fv := flags.(*DeviceMonitorFlags)
	config, err := ParseConfig(ctx, fv.ConfigFlags)
	if err != nil {
		return err
	}
	lf, err := newLogfile(fv.LogFile)
	if err != nil {
		return err
	}
	defer lf.Close()
	l, err := NewLogger(lf, nil)
	if err != nil {
		return err
	}
	monitors := []func() error{}
	if fv.Ping {
		monitors = append(monitors, func() error {
			return d.pingMonitor(ctx, fv.DryRun, config, l)
		})
	}
	if fv.ARP {
		monitors = append(monitors, func() error {
			return d.arpMonitor(ctx, fv.DryRun, config, l)
		})
	}
	if fv.RTSP {
		monitors = append(monitors, func() error {
			return d.rtspMonitor(ctx, fv.DryRun, config, l)
		})
	}
	if fv.Routing {
		monitors = append(monitors, func() error {
			return d.routeMonitor(ctx, fv.DryRun, config, l)
		})
	}
	if fv.Syslog {
		monitors = append(monitors, func() error {
			return d.syslogMonitor(ctx, fv.DryRun, config, l)
		})
	}
	if fv.CGI {
		monitors = append(monitors, func() error {
			return d.cgiMonitor(ctx, fv.DryRun, config, l)
		})
	}
	var g errgroup.T
	for _, m := range monitors {
		g.Go(m)
	}
	return g.Wait()
}

func (d *Devices) pingMonitor(ctx context.Context, dryRun bool, config *Config, l *Logger) error {
	devs, err := config.ICMPDevices()
	if err != nil {
		return err
	}
	if len(devs) == 0 {
		return nil
	}
	if dryRun {
		fmt.Printf("ping %d devices with interval %s and timeout %s\n", len(devs), config.Options.ICMP.Interval, config.Options.ICMP.Timeout)
		for _, dev := range devs {
			fmt.Printf("ping %s\n", dev.ipAddr)
		}
		return nil
	}
	monitor := NewICMPMonitor(l)
	return monitor.MonitorAll(ctx, devs)
}

func (d *Devices) arpMonitor(ctx context.Context, dryRun bool, config *Config, l *Logger) error {
	devs, err := config.ARPDevices()
	if err != nil {
		return err
	}
	if len(devs) == 0 {
		return nil
	}
	if dryRun {
		fmt.Printf("arp %d devices with interval %s\n", len(devs), config.Options.ARP.Interval)
		for _, dev := range devs {
			fmt.Printf("arp %s\n", dev.ipAddr)
		}
		return nil
	}
	monitor := NewARPMonitor(l, config.Options.ARP.Interval)
	return monitor.MonitorAll(ctx, devs)
}

func (d *Devices) rtspMonitor(ctx context.Context, dryRun bool, config *Config, l *Logger) error {
	devs, err := config.RTSPDevices()
	if err != nil {
		return err
	}
	if len(devs) == 0 {
		return nil
	}
	if dryRun {
		fmt.Printf("rtsp %d devices with interval %s\n", len(devs), config.Options.RTSP.Interval)
		for _, dev := range devs {
			fmt.Printf("rtsp %s\n", dev.ipAddr)
		}
		return nil
	}
	monitor := NewRTSPMonitor(l)
	return monitor.MonitorAll(ctx, devs)
}

func (d *Devices) routeMonitor(ctx context.Context, dryRun bool, config *Config, l *Logger) error {
	devs, err := config.RoutingDevices()
	if err != nil {
		return err
	}
	if len(devs) == 0 {
		return nil
	}
	if dryRun {
		fmt.Printf("route table monitoring with interval %s\n", config.Options.Routing.Interval)
		for _, dev := range devs {
			fmt.Printf("route %s\n", dev.ipAddr)
		}
		return nil
	}
	monitor := NewRouteMonitor(l, config.Options.Routing.Interval)
	return monitor.MonitorAll(ctx, devs)
}

func (d *Devices) syslogMonitor(ctx context.Context, dryRun bool, config *Config, l *Logger) error {
	if dryRun {
		fmt.Printf("syslog server\n")
	}
	s := newSyslogServer(l)
	return s.run(ctx)
}

func (d *Devices) cgiMonitor(ctx context.Context, dryRun bool, config *Config, l *Logger) error {
	cgiInvocations, err := config.CGIInvocations()
	if err != nil {
		return err
	}
	if len(cgiInvocations) == 0 {
		return nil
	}
	if dryRun {
		fmt.Printf("cgi %d devices with interval %s and timeout %s\n", len(cgiInvocations), config.Options.CGI.Interval, config.Options.CGI.Timeout)
		for _, inv := range cgiInvocations {
			fmt.Printf("cgi %s %s\n", inv.IPAddr, inv.Path)
		}
		return nil
	}
	return NewCGIMonitor(l).MonitorAll(ctx, cgiInvocations)
}
