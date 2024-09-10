package main

import (
	"context"

	"cloudeng.io/sync/errgroup"
	"gopkg.in/mcuadros/go-syslog.v2"
	"gopkg.in/mcuadros/go-syslog.v2/format"
)

type syslogServer struct {
	l *Logger
}

func newSyslogServer(l *Logger) *syslogServer {
	return &syslogServer{l: l}
}

func (s *syslogServer) log(ctx context.Context, format string, args []any) {
	s.l.Log(ctx, "syslog", format, args...)
}

func kv(parts format.LogParts) []any {
	var res []any
	for k, v := range parts {
		res = append(res, k, v)
	}
	return res
}

func (s *syslogServer) run(ctx context.Context) error {
	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)

	server := syslog.NewServer()
	server.SetFormat(syslog.RFC3164) // How to support other formats?
	server.SetHandler(handler)
	server.ListenUDP("0.0.0.0:514")
	server.Boot()

	var g errgroup.T

	g.Go(func() error {
		for {
			select {
			case logParts, ok := <-channel:
				if !ok {
					return nil
				}
				s.log(ctx, "received syslog", kv(logParts))
			case <-ctx.Done():
				server.Kill()
				close(channel)
				return ctx.Err()
			}
		}
	})
	g.Go(func() error {
		server.Wait()
		return nil
	})
	return g.Wait()
}
