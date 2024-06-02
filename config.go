package main

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"cloudeng.io/cmdutil/cmdyaml"
)

type AuthConfig struct {
	ID    string `yaml:"auth_id"`
	User  string `yaml:"user"`
	Token string `yaml:"token"`
}

func (a AuthConfig) String() string {
	return a.ID + "[" + a.User + "]	"
}

type Device struct {
	Name string      `yaml:"name"`
	IP   string      `yaml:"ip"`
	RTSP *RTSPConfig `yaml:"rtsp,omitempty"`
	ICMP *ICMPConfig `yaml:"icmp,omitempty"`
}

type ICMPConfig struct {
	Period  time.Duration `yaml:"period,omitempty"`
	Timeout time.Duration `yaml:"timeout,omitempty"`
}

type RTSPConfig struct {
	Path    string        `yaml:"path,omitempty"`
	AuthID  string        `yaml:"auth_id,omitempty"`
	Port    int           `yaml:"port,omitempty"`
	Media   string        `yaml:"media,omitempty"`
	Period  time.Duration `yaml:"period,omitempty"`
	Timeout time.Duration `yaml:"timeout,omitempty"`
}

func (r RTSPConfig) String() string {
	out := strings.Builder{}
	fmt.Fprintf(&out, "period: %v, path: %s ", r.Period, r.Path)
	if len(r.AuthID) > 0 {
		fmt.Fprintf(&out, "(auth_id: %v)", r.AuthID)
	}
	return out.String()
}

func (d Device) String() string {
	out := strings.Builder{}
	fmt.Fprintf(&out, "%s[%s]	", d.Name, d.IP)
	if d.RTSP != nil {
		fmt.Fprintf(&out, "    RTSP: %v\n", d.RTSP)
	}
	return out.String()
}

type ICMPOption struct {
	Devices []string      `yaml:"devices"`
	Period  time.Duration `yaml:"period,omitempty"`
	Timeout time.Duration `yaml:"timeout,omitempty"`
}

func (i ICMPOption) String() string {
	return fmt.Sprintf("period: %v", i.Period)
}

type RTSPOption struct {
	Devices []string      `yaml:"devices"`
	Period  time.Duration `yaml:"period,omitempty"`
	Timeout time.Duration `yaml:"timeout,omitempty"`
}

type ARPOption struct {
	Devices []string      `yaml:"devices"`
	Period  time.Duration `yaml:"period,omitempty"`
}

type RoutingOption struct {
	Devices []string      `yaml:"devices"`
	Period  time.Duration `yaml:"period,omitempty"`
}

type Options struct {
	ICMP    *ICMPOption    `yaml:"icmp"`
	RTSP    *RTSPOption    `yaml:"rtsp"`
	ARP     *ARPOption     `yaml:"arp"`
	Routing *RoutingOption `yaml:"routing"`
}

type Config struct {
	Options Options  `yaml:"options"`
	Devices []Device `yaml:"devices"`
	auth    map[string]*AuthConfig
	devices map[string]*Device
}

type ConfigFlags struct {
	AuthFile    string `subcmd:"auth,$HOME/.netmon-auth.yaml,auth config file to use"`
	DevicesFile string `subcmd:"devices,$HOME/.netmon-config.yaml,config file to use"`
}

func ParseConfig(ctx context.Context, flags ConfigFlags) (*Config, error) {
	var config Config
	var auth []AuthConfig
	err := cmdyaml.ParseConfigFile(ctx, flags.AuthFile, &auth)
	if err != nil {
		return nil, err
	}
	err = cmdyaml.ParseConfigFile(ctx, flags.DevicesFile, &config)
	if err != nil {
		return nil, err
	}
	config.auth = make(map[string]*AuthConfig)
	for i := range auth {
		config.auth[auth[i].ID] = &auth[i]
	}
	config.devices = make(map[string]*Device)
	for i := range config.Devices {
		config.devices[config.Devices[i].Name] = &config.Devices[i]
	}
	return &config, err
}

type ICMPDevice struct {
	Name    string
	IP      string
	Period  time.Duration
	Timeout time.Duration
	ipAddr  netip.Addr
}

func (c Config) deviceNamesFor(names []string) []string {
	if len(names) == 0 || (len(names) == 1 && names[0] == "all") {
		names = names[:0]
		for _, d := range c.Devices {
			names = append(names, d.Name)
		}
	}
	return names
}

func (c Config) ICMPDevices() ([]ICMPDevice, error) {
	if c.Options.ICMP == nil {
		return nil, nil
	}
	names := c.deviceNamesFor(c.Options.ICMP.Devices)
	cfg := make([]ICMPDevice, 0, len(names))
	for _, name := range names {
		d := c.devices[name]
		if d == nil {
			return nil, fmt.Errorf("device %q not found\n", name)
		}
		addr, err := ParseIPAddr(d.IP)
		if err != nil {
			return nil, fmt.Errorf("device %q: %v", name, err)
		}
		v := ICMPDevice{
			Name:    d.Name,
			IP:      d.IP,
			Period:  c.Options.ICMP.Period,
			Timeout: c.Options.ICMP.Timeout,
			ipAddr:  addr,
		}
		if d.ICMP != nil {
			if d.ICMP.Period != 0 {
				v.Period = d.ICMP.Period
			}
			if d.ICMP.Timeout != 0 {
				v.Timeout = d.ICMP.Timeout
			}
		}
		if v.Timeout == 0 {
			v.Timeout = 5 * time.Second
		}
		cfg = append(cfg, v)
	}
	return cfg, nil
}

type RTSPDevice struct {
	Name    string
	IP      string
	Port    int
	URL     string
	SafeURL string // no password
	Media   string
	Period  time.Duration
	Timeout time.Duration
	ipAddr  netip.Addr
}

func (c Config) RTSPDevices() ([]RTSPDevice, error) {
	if c.Options.RTSP == nil {
		return nil, nil
	}
	names := c.deviceNamesFor(c.Options.RTSP.Devices)
	cfg := make([]RTSPDevice, 0, len(names))
	for _, name := range names {
		d := c.devices[name]
		if d == nil {
			fmt.Printf("device %q not found\n", name)
			continue
		}
		if d.RTSP == nil {
			continue
		}
		addr, err := ParseIPAddr(d.IP)
		if err != nil {
			return nil, fmt.Errorf("device %q: %v", name, err)
		}
		v := RTSPDevice{
			Name:    d.Name,
			IP:      d.IP,
			Period:  c.Options.RTSP.Period,
			Timeout: c.Options.RTSP.Timeout,
			Media:   "H264",
			Port:    554,
			ipAddr:  addr,
		}
		if d.RTSP.Port != 0 {
			v.Port = d.RTSP.Port
		}
		if d.RTSP.Period != 0 {
			v.Period = d.RTSP.Period
		}
		if d.RTSP.Timeout != 0 {
			v.Timeout = d.RTSP.Timeout
		}
		if len(d.RTSP.Media) != 0 {
			v.Media = d.RTSP.Media
		}
		auth := c.auth[d.RTSP.AuthID]
		if auth == nil {
			auth = &AuthConfig{User: "admin", Token: "admin"}
		}
		v.URL = fmt.Sprintf("rtsp://%s:%s@%s:%d/%s", auth.User, auth.Token, v.IP, v.Port, d.RTSP.Path)
		v.SafeURL = fmt.Sprintf("rtsp://%s:%s@%s:%d/%s", auth.User, "****", v.IP, v.Port, d.RTSP.Path)
		if v.Timeout == 0 {
			v.Timeout = 5 * time.Second
		}
		cfg = append(cfg, v)
	}
	return cfg, nil
}

func (c Config) devicesFor(names []string) ([]Device, error) {
	cfg := make([]Device, 0, len(names))
	for _, name := range names {
		d := c.devices[name]
		if d == nil {
			fmt.Printf("device %q not found\n", name)
			continue
		}
		cfg = append(cfg, *d)
	}
	return cfg, nil
}

func (c Config) ARPDevices() ([]Device, error) {
	if c.Options.ARP == nil {
		return nil, nil
	}
	names := c.deviceNamesFor(c.Options.ARP.Devices)
	return c.devicesFor(names)
}

func (c Config) RoutingDevices() ([]Device, error) {
	if c.Options.Routing == nil {
		return nil, nil
	}
	names := c.deviceNamesFor(c.Options.Routing.Devices)
	return c.devicesFor(names)
}
