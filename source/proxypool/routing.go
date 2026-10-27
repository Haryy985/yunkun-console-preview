package proxypool

import (
	"context"
	"errors"
	"net"
	"time"
)

type proxyDialError struct{ cause error }

func (e *proxyDialError) Error() string { return "proxy connection failed" }
func (e *proxyDialError) Unwrap() error { return e.cause }

var errProxyAuthentication = errors.New("proxy authentication failed")

type markedProxyDialer struct{ *net.Dialer }

func (d *markedProxyDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

func (d *markedProxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := d.Dialer.DialContext(ctx, network, address)
	if err != nil && ctx.Err() == nil {
		return nil, &proxyDialError{err}
	}
	return conn, err
}

func (g *Gateway) selectRoute(ctx context.Context, taskID int64) (Route, error) {
	var route Route
	var err error
	if g.Select != nil {
		route, err = g.Select(ctx, taskID)
	} else {
		route, err = g.Resolve(ctx, taskID)
	}
	if err == nil && g.Validate != nil {
		err = g.Validate(ctx, route)
	}
	return route, err
}

func (g *Gateway) validateRoute(ctx context.Context, route Route) error {
	if g.Validate != nil {
		return g.Validate(ctx, route)
	}
	current, err := g.Resolve(ctx, route.TaskID)
	if err != nil || current.Upstream == nil || route.Upstream == nil || current.Revision != route.Revision ||
		current.Node.ID != route.Node.ID || current.Upstream.String() != route.Upstream.String() {
		return errRouteUnavailable
	}
	return nil
}

func (g *Gateway) watchRoute(ctx context.Context, cancel context.CancelFunc, route Route) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var serverDone <-chan struct{}
		if g.Context != nil {
			serverDone = g.Context.Done()
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-serverDone:
				cancel()
				return
			case <-ticker.C:
				if g.validateRoute(ctx, route) != nil {
					cancel()
					return
				}
			}
		}
	}()
}

// Only failures attributable to the proxy itself change pool health. Target
// status codes, target TLS errors, cancellation and ambiguous I/O do not.
func (g *Gateway) reportProxyFailure(ctx context.Context, route Route, err error) bool {
	if ctx.Err() != nil || g.Failure == nil {
		return false
	}
	var dial *proxyDialError
	reason := ""
	if errors.As(err, &dial) {
		reason = "proxy_connect_failed"
	} else if errors.Is(err, errProxyAuthentication) {
		reason = "proxy_authentication_failed"
	}
	if reason == "" {
		return false
	}
	return g.Failure(route, reason) == nil
}
