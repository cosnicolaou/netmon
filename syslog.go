package main

import (
	"context"
	"fmt"

	"cloudeng.io/sync/errgroup"
	"gopkg.in/mcuadros/go-syslog.v2"
)

type syslogServer struct {
	l *Logger
}

func newSyslogServer(l *Logger) *syslogServer {
	return &syslogServer{l: l}
}

func (s *syslogServer) run(ctx context.Context) error {
	channel := make(syslog.LogPartsChannel)
	handler := syslog.NewChannelHandler(channel)

	server := syslog.NewServer()
	server.SetFormat(syslog.RFC5424)
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
				fmt.Println(logParts)
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
