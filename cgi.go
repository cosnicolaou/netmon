package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"

	"cloudeng.io/sync/errgroup"
	"github.com/icholy/digest"
)

type CGIMonitor struct {
	l *Logger
}

func NewCGIMonitor(l *Logger) *CGIMonitor {
	return &CGIMonitor{l: l}
}

type perHostState struct {
	mu  sync.Mutex
	jar *cookiejar.Jar
}

func (h *perHostState) Lock() {
	h.mu.Lock()
}

func (h *perHostState) Unlock() {
	h.mu.Unlock()
}

func (s *CGIMonitor) MonitorAll(ctx context.Context, invocations []CGIInvocation) error {
	// Group by host IP
	perHost := map[string]*perHostState{}
	for _, invocation := range invocations {
		k := invocation.IPAddr.String()
		if _, ok := perHost[k]; !ok {
			jar, _ := cookiejar.New(nil)
			perHost[k] = &perHostState{
				jar: jar,
			}
		}
	}

	var g errgroup.T
	for _, invocation := range invocations {
		k := invocation.IPAddr.String()
		r := &cgiGet{config: invocation, hostState: perHost[k], l: s.l}
		g.Go(func() error {
			return r.issueCalls(ctx)
		})
	}
	return g.Wait()
}

type cgiGet struct {
	config    CGIInvocation
	hostState *perHostState
	l         *Logger
}

func (c *cgiGet) log(ctx context.Context, format string, args ...any) {
	c.l.Log(ctx, "cgi", format, args...)
}

func (c *cgiGet) warn(ctx context.Context, format string, args ...any) {
	c.l.Warn(ctx, "cgi", format, args...)
}

func (c *cgiGet) issueCalls(ctx context.Context) error {
	for {
		inv := c.config
		url := fmt.Sprintf("%s://%s:%d/%s", inv.Scheme, inv.IPAddr.String(), inv.Port, inv.Path)
		if err := c.call(ctx, url, inv); err != nil {
			if errors.Is(err, context.Canceled) {
				c.warn(ctx, "exiting", "name", inv.Name, "url", url, "err", ctx.Err())
				return err
			}
			c.warn(ctx, "call failed", "name", inv.Name, "url", url, "err", err)
		}
		if inv.OnceOnly {
			return nil
		}
		select {
		case <-ctx.Done():
			c.warn(ctx, "exiting", "name", inv.Name, "url", url, "err", ctx.Err())
			return ctx.Err()
		case <-time.After(inv.Interval):
		}

	}
}

func (c *cgiGet) call(ctx context.Context, url string, inv CGIInvocation) error {
	c.hostState.Lock()
	defer c.hostState.Unlock()
	ctx, cancel := context.WithTimeout(ctx, inv.Timeout)
	defer cancel()
	client := &http.Client{
		Transport: &digest.Transport{
			Jar:      c.hostState.jar,
			Username: inv.Auth.User,
			Password: inv.Auth.Token,
		},
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			c.log(ctx, "timeout", "name", inv.Name, "url", "timeout", inv.Timeout, url, "err", err)
			return nil
		}
		return err
	}
	defer res.Body.Close()
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	c.log(ctx, "ok", "name", inv.Name, "url", url, "body", string(buf))
	return nil
}
