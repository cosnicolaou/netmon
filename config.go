package main

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"cloudeng.io/cmdutil/cmdyaml"
	"cloudeng.io/cmdutil/keystore"
	"cloudeng.io/macos/keychainfs"
)

const (
	DefaultICMPTimeout      = 5 * time.Second
	DefaultICMPPingInterval = 5 * time.Second

	DefaultRTSPTimeout  = 5 * time.Second
	DefaultRTSPInterval = 30 * time.Second
	DefaultRSTPPort     = 554

	DefaultCGITimeout  = 5 * time.Second
	DefaultCGIInterval = time.Minute
	DefaultCGIPort     = 80

	DefaultARPInterval = 10 * time.Second
)

type Device struct {
	Name   string      `yaml:"name"`
	IP     string      `yaml:"ip"`
	AuthID string      `yaml:"key_id,omitempty"`
	RTSP   *RTSPConfig `yaml:"rtsp,omitempty"`
	ICMP   *ICMPConfig `yaml:"icmp,omitempty"`
	CGI    []CGIConfig `yaml:"cgi,omitempty"`
	ipAddr netip.Addr
}

type ICMPConfig struct {
	Interval time.Duration `yaml:"interval,omitempty"`
	Timeout  time.Duration `yaml:"timeout,omitempty"`
}

type RTSPConfig struct {
	Path     string        `yaml:"path,omitempty"`
	AuthID   string        `yaml:"key_id,omitempty"`
	Port     int           `yaml:"port,omitempty"`
	Media    string        `yaml:"media,omitempty"`
	Interval time.Duration `yaml:"interval,omitempty"`
	Timeout  time.Duration `yaml:"timeout,omitempty"`
}

type CGIConfig struct {
	Path     string        `yaml:"path,omitempty"`
	Scheme   string        `yaml:"scheme,omitempty"`
	Port     int           `yaml:"port,omitempty"`
	Timeout  time.Duration `yaml:"timeout,omitempty"`
	Interval time.Duration `yaml:"interval,omitempty"`
	OnceOnly bool          `yaml:"once_only,omitempty"`
	AuthID   string        `yaml:"key_id,omitempty"`
}

func (r RTSPConfig) String() string {
	out := strings.Builder{}
	fmt.Fprintf(&out, "interval: %v, path: %s ", r.Interval, r.Path)
	if len(r.AuthID) > 0 {
		fmt.Fprintf(&out, "(key_id: %v)", r.AuthID)
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
	Devices  []string      `yaml:"devices"`
	Interval time.Duration `yaml:"interval,omitempty"`
	Timeout  time.Duration `yaml:"timeout,omitempty"`
}

func (i ICMPOption) String() string {
	return fmt.Sprintf("interval: %v", i.Interval)
}

type RTSPOption struct {
	Devices  []string      `yaml:"devices"`
	Interval time.Duration `yaml:"interval,omitempty"`
	Timeout  time.Duration `yaml:"timeout,omitempty"`
}

type ARPOption struct {
	Devices  []string      `yaml:"devices"`
	Interval time.Duration `yaml:"interval,omitempty"`
}

type RoutingOption struct {
	Devices  []string      `yaml:"devices"`
	Interval time.Duration `yaml:"interval,omitempty"`
}

type CGIOption struct {
	Interval time.Duration `yaml:"interval,omitempty"`
	Timeout  time.Duration `yaml:"timeout,omitempty"`
}

type Options struct {
	ICMP    *ICMPOption    `yaml:"icmp"`
	RTSP    *RTSPOption    `yaml:"rtsp"`
	ARP     *ARPOption     `yaml:"arp"`
	Routing *RoutingOption `yaml:"routing"`
	CGI     *CGIOption     `yaml:"cgi"`
}

type Config struct {
	Options Options  `yaml:"options"`
	Devices []Device `yaml:"devices"`
	auth    keystore.Keys
	devices map[string]*Device
}

type ConfigFlags struct {
	AuthFile    string `subcmd:"auth,keychain:///netmon-auth.yaml?account=,auth config file to use"`
	DevicesFile string `subcmd:"devices,$HOME/.netmon-config.yaml,config file to use"`
}

var uriHandlers = map[string]cmdyaml.URLHandler{
	"keychain": keychainfs.NewSecureNoteFSFromURL,
}

func ParseConfig(ctx context.Context, flags ConfigFlags) (*Config, error) {
	var config Config
	keys, err := keystore.ParseConfigURI(ctx, flags.AuthFile, uriHandlers)
	if err != nil {
		return nil, err
	}
	err = cmdyaml.ParseConfigFile(ctx, flags.DevicesFile, &config)
	if err != nil {
		return nil, err
	}
	config.auth = keys
	config.devices = make(map[string]*Device)
	for i := range config.Devices {
		device := &config.Devices[i]
		if len(device.IP) > 0 {
			device.ipAddr, err = ParseIPAddr(device.IP)
			if err != nil {
				return nil, fmt.Errorf("device %q: %v", device.Name, err)
			}
		}
		config.devices[config.Devices[i].Name] = device
	}
	return &config, err
}

type ICMPDevice struct {
	Name     string
	IP       string
	Interval time.Duration
	Timeout  time.Duration
	ipAddr   netip.Addr
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

func defaultIntervalTimeout(interval, timeout, defaultInterval, defaultTimeout time.Duration) (time.Duration, time.Duration) {
	if interval == 0 {
		interval = defaultInterval
	}
	if timeout == 0 {
		timeout = defaultTimeout
	}
	return interval, timeout
}

func (c Config) defaultAuthID(authID, defaultAuthID string) keystore.KeyInfo {
	if auth, ok := c.auth[authID]; ok {
		return auth
	}
	if auth, ok := c.auth[defaultAuthID]; ok {
		return auth
	}
	return keystore.KeyInfo{User: "admin", Token: "admin"}
}

func defaultPort(port, defaultPort int) int {
	if port == 0 {
		port = defaultPort
	}
	return port
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
		v := ICMPDevice{
			Name:   d.Name,
			IP:     d.IP,
			ipAddr: d.ipAddr,
		}
		v.Interval, v.Timeout = defaultIntervalTimeout(d.ICMP.Interval, d.ICMP.Timeout, c.Options.ICMP.Interval, c.Options.ICMP.Timeout)
		v.Interval, v.Timeout = defaultIntervalTimeout(v.Interval, v.Timeout, DefaultICMPTimeout, DefaultICMPPingInterval)
		cfg = append(cfg, v)
	}
	return cfg, nil
}

type RTSPDevice struct {
	Name     string
	IP       string
	Port     int
	URL      string
	SafeURL  string // no password
	Media    string
	Interval time.Duration
	Timeout  time.Duration
	ipAddr   netip.Addr
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
		v := RTSPDevice{
			Name:   d.Name,
			IP:     d.IP,
			Media:  "H264",
			ipAddr: d.ipAddr,
		}
		if len(d.RTSP.Media) != 0 {
			v.Media = d.RTSP.Media
		}
		v.Port = defaultPort(d.RTSP.Port, DefaultRSTPPort)
		v.Interval, v.Timeout = defaultIntervalTimeout(d.RTSP.Interval, d.RTSP.Timeout, c.Options.RTSP.Interval, c.Options.RTSP.Timeout)
		v.Timeout, v.Interval = defaultIntervalTimeout(v.Interval, v.Timeout, DefaultRTSPTimeout, DefaultRTSPInterval)
		auth := c.defaultAuthID(d.RTSP.AuthID, d.AuthID)
		v.URL = fmt.Sprintf("rtsp://%s:%s@%s:%d/%s", auth.User, auth.Token, v.IP, v.Port, d.RTSP.Path)
		v.SafeURL = fmt.Sprintf("rtsp://%s:%s@%s:%d/%s", auth.User, "****", v.IP, v.Port, d.RTSP.Path)
		cfg = append(cfg, v)
	}
	return cfg, nil
}

type CGIInvocation struct {
	Name     string
	Scheme   string
	Path     string
	Port     int
	Interval time.Duration
	Timeout  time.Duration
	OnceOnly bool
	Auth     keystore.KeyInfo
	IPAddr   netip.Addr
}

func (c Config) CGIInvocations() ([]CGIInvocation, error) {
	if c.Options.CGI == nil {
		return nil, nil
	}
	invocations := make([]CGIInvocation, 0, len(c.devices))
	for _, device := range c.devices {
		if device.CGI == nil {
			continue
		}
		for _, invocation := range device.CGI {
			v := CGIInvocation{
				Name:     device.Name,
				Path:     invocation.Path,
				IPAddr:   device.ipAddr,
				Scheme:   invocation.Scheme,
				OnceOnly: invocation.OnceOnly,
			}
			if v.Scheme == "" {
				v.Scheme = "http"
			}
			v.Port = defaultPort(v.Port, DefaultCGIPort)
			v.Interval, v.Timeout = defaultIntervalTimeout(invocation.Interval, invocation.Timeout, c.Options.CGI.Interval, c.Options.CGI.Timeout)
			v.Interval, v.Timeout = defaultIntervalTimeout(v.Interval, v.Timeout, DefaultCGIInterval, DefaultCGITimeout)
			v.Auth = c.defaultAuthID(invocation.AuthID, device.AuthID)
			invocations = append(invocations, v)
		}
	}
	return invocations, nil
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
