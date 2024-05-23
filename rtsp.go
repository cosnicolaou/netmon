package main

import (
	"context"
	"fmt"
	"time"

	"cloudeng.io/sync/errgroup"
	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph265"
	"github.com/pion/rtp"
)

type RTSPMonitor struct {
	l *Logger
}

func NewRTSPMonitor(l *Logger) *RTSPMonitor {
	return &RTSPMonitor{l: l}
}

func (m *RTSPMonitor) log(ctx context.Context, format string, args ...any) {
	m.l.Log(ctx, "rtsp", format, args...)
}

func (m *RTSPMonitor) warn(ctx context.Context, format string, args ...any) {
	m.l.Warn(ctx, "rtsp", format, args...)
}

func (m *RTSPMonitor) MonitorAll(ctx context.Context, devs []RTSPDevice) error {
	var g errgroup.T
	for _, dev := range devs {
		g.Go(func() error {
			return m.MonitorDevice(ctx, dev)
		})
	}
	return g.Wait()
}

func (m *RTSPMonitor) MonitorDevice(ctx context.Context, dev RTSPDevice) error {
	for {
		m.log(ctx, "connecting", "name", dev.Name, "url", dev.SafeURL, "media", dev.Media)
		stream, err := m.connect(ctx, dev)
		if err != nil {
			m.warn(ctx, "failed to connect", "name", dev.Name, "url", dev.SafeURL, "media", dev.Media, "err", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(dev.Period):
			}
			continue
		}
		m.log(ctx, "connected", "name", dev.Name, "url", dev.SafeURL, "media", dev.Media)
		if err := stream.sink(ctx, time.Second*10); err != nil {
			m.warn(ctx, "playback ended", "name", dev.Name, "url", dev.SafeURL, "media", dev.Media, "err", err)
		}
		stream.close()
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

type rtspStream struct {
	m      *RTSPMonitor
	client *gortsplib.Client
	decode func(ctx context.Context, decode func(pkt *rtp.Packet) ([][]byte, error), pkt *rtp.Packet) error
	media  *description.Media
	format format.Format
	dev    RTSPDevice
	pktPTS chan time.Duration
}

func (m *RTSPMonitor) connect(ctx context.Context, dev RTSPDevice) (*rtspStream, error) {
	c := &gortsplib.Client{}

	u, err := base.ParseURL(dev.URL)
	if err != nil {
		return nil, err
	}

	err = c.Start(u.Scheme, u.Host)
	if err != nil {
		return nil, err
	}

	desc, _, err := c.Describe(u)
	if err != nil {
		return nil, err
	}

	stream := &rtspStream{
		m:      m,
		client: c,
		dev:    dev,
		pktPTS: make(chan time.Duration, 1000),
	}

	var decodePkt func(pkt *rtp.Packet) ([][]byte, error)
	if dev.Media == "" || dev.Media == "H264" {
		h264 := &format.H264{}
		media := desc.FindFormat(&h264)
		if media == nil {
			return nil, fmt.Errorf("H264 not supported")
		}
		decoder, err := h264.CreateDecoder()
		if err != nil {
			return nil, err
		}
		stream.decode = stream.h264Decode
		stream.media = media
		stream.format = h264
		decodePkt = decoder.Decode
	}
	if dev.Media == "H265" {
		h265 := &format.H265{}
		media := desc.FindFormat(&h265)
		if media == nil {
			return nil, fmt.Errorf("H265 not supported")
		}
		decoder, err := h265.CreateDecoder()
		if err != nil {
			return nil, err
		}
		stream.decode = stream.h265Decode
		stream.media = media
		stream.format = h265
		decodePkt = decoder.Decode
	}

	_, err = c.Setup(desc.BaseURL, stream.media, 0, 0)
	if err != nil {
		panic(err)
	}

	// called when a RTP packet arrives
	c.OnPacketRTP(stream.media, stream.format, func(pkt *rtp.Packet) {
		stream.callback(ctx, decodePkt, pkt)
	})

	return stream, nil
}

func (s *rtspStream) h264Decode(ctx context.Context, decode func(*rtp.Packet) ([][]byte, error), pkt *rtp.Packet) error {
	_, err := decode(pkt)
	if err != nil {
		if err != rtph264.ErrNonStartingPacketAndNoPrevious && err != rtph264.ErrMorePacketsNeeded {
			s.m.warn(ctx, "H264 packet decoder error", "name", s.dev.Name, "url", s.dev.SafeURL, "err", err)
			return err
		}
	}
	return nil
}

func (s *rtspStream) h265Decode(ctx context.Context, decode func(*rtp.Packet) ([][]byte, error), pkt *rtp.Packet) error {
	_, err := decode(pkt)
	if err != nil {
		if err != rtph265.ErrNonStartingPacketAndNoPrevious && err != rtph265.ErrMorePacketsNeeded {
			s.m.warn(ctx, "H265 packet decoder error", "name", s.dev.Name, "url", s.dev.SafeURL, "err", err)
			return err
		}
	}
	return nil
}

func (s *rtspStream) callback(ctx context.Context, decode func(*rtp.Packet) ([][]byte, error), pkt *rtp.Packet) {
	ntp, ok := s.client.PacketPTS(s.media, pkt)
	if !ok {
		s.m.warn(ctx, "waiting for timestamp", "name", s.dev.Name, "url", s.dev.SafeURL)
		return
	}
	s.decode(ctx, decode, pkt)
	select {
	case s.pktPTS <- ntp:
	case <-ctx.Done():
		return
	default:
	}
}

func (s *rtspStream) sink(ctx context.Context, progressDurationSecs time.Duration) error {
	resp, err := s.client.Play(nil)
	if err != nil {
		return fmt.Errorf("play failed: %v", err)
	}
	if resp.StatusCode != base.StatusOK {
		return fmt.Errorf("play failed: waiting for timestamp: %v", resp.StatusMessage)
	}
	last := 0 * time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case pts := <-s.pktPTS:
			if n := pts.Round(progressDurationSecs); n > last {
				last = n
				s.m.log(ctx, "ok", "name", s.dev.Name, "url", s.dev.SafeURL, "pts", pts.String())
			}
		case <-time.After(s.dev.Period):
			return fmt.Errorf("timeout after %s", s.dev.Period)
		}
	}
}

func (s *rtspStream) close() {
	s.client.Close()
	close(s.pktPTS)
}
