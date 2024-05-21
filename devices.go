package main

import (
	"context"

	"cloudeng.io/sync/errgroup"
)

type DeviceMonitorFlags struct {
	ConfigFlags
	LogFile string `subcmd:"log-file,netmon.slog,the file to write structured logs to"`
	Ping    bool   `subcmd:"ping,true,enable pinging of devices"`
	ARP     bool   `subcmd:"arp,true,enable arp monitoring"`
	RTSP    bool   `subcmd:"rtsp,false,enable rtsp monitoring"`
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
			return d.pingMonitor(ctx, config, l)
		})
	}
	if fv.ARP {
		monitors = append(monitors, func() error {
			return d.arpMonitor(ctx, config, l)
		})
	}
	if fv.RTSP {
		monitors = append(monitors, func() error {
			return d.rtspMonitor(ctx, config, l)
		})

	}
	var g errgroup.T
	for _, m := range monitors {
		g.Go(m)
	}
	return g.Wait()
}

func (d *Devices) pingMonitor(ctx context.Context, config *Config, l *Logger) error {
	devs, err := config.ICMPDevices()
	if err != nil {
		return err
	}
	if len(devs) == 0 {
		return nil
	}
	monitor := NewICMPMonitor(l)
	return monitor.MonitorAll(ctx, devs)
}

func (d *Devices) arpMonitor(ctx context.Context, config *Config, l *Logger) error {
	devs, err := config.ARPDevices()
	if err != nil {
		return err
	}
	if len(devs) == 0 {
		return nil
	}
	monitor := NewARPMonitor(l, config.Options.ARP.Period)
	return monitor.MonitorAll(ctx, devs)
}

func (d *Devices) rtspMonitor(ctx context.Context, config *Config, l *Logger) error {
	devs, err := config.RTSPDevices()
	if err != nil {
		return err
	}
	if len(devs) == 0 {
		return nil
	}
	monitor := NewRTSPMonitor(l)
	return monitor.MonitorAll(ctx, devs)
}
