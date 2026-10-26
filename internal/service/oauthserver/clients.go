package oauthserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

func validateClient(c Client) error {
	if len(c.Name) == 0 || len(c.Name) > 160 || len(c.RedirectURIs) == 0 || len(c.RedirectURIs) > 8 || (c.TokenEndpointAuthMethod != "" && c.TokenEndpointAuthMethod != "none") {
		return ErrInvalid
	}
	for _, g := range c.GrantTypes {
		if g != "authorization_code" && g != "refresh_token" {
			return ErrInvalid
		}
	}
	for _, r := range c.ResponseTypes {
		if r != "code" {
			return ErrInvalid
		}
	}
	for _, r := range c.RedirectURIs {
		u, e := url.Parse(r)
		if e != nil || len(r) > 2048 || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && loopbackHost(u.Hostname()))) {
			return ErrInvalid
		}
	}
	return nil
}

type cachedClient struct {
	Client  Client
	Expires int64
}

func (s *Service) Client(ctx context.Context, id string) (Client, error) {
	if len(id) > 500 || id == "" {
		return Client{}, ErrInvalid
	}
	for _, c := range s.cfg.Clients {
		if c.ID == id {
			if e := validateClient(c); e != nil {
				return Client{}, e
			}
			return c, nil
		}
	}
	r, e := s.store.Get(ctx, "client:"+id)
	if e == nil {
		var c cachedClient
		if json.Unmarshal(r.Payload, &c) == nil && (c.Expires == 0 || c.Expires > time.Now().Unix()) {
			return c.Client, nil
		}
	}
	u, e := url.Parse(id)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || (u.Port() != "" && u.Port() != "443") {
		return Client{}, ErrInvalid
	}
	s.metadataMu.Lock()
	if time.Now().Before(s.metadataNext) {
		s.metadataMu.Unlock()
		return Client{}, ErrInvalid
	}
	s.metadataNext = time.Now().Add(time.Second)
	s.metadataMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	// Resolve and pin the actual dial target. Redirects and proxy settings are
	// deliberately disabled so rebinding cannot bypass the public-address check.
	transport := &http.Transport{MaxResponseHeaderBytes: 16384, TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: 2 * time.Second, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if e != nil || len(ips) == 0 {
			return nil, ErrInvalid
		}
		for _, ip := range ips {
			if !publicAddress(ip) {
				return nil, ErrInvalid
			}
		}
		var last error
		for _, ip := range ips {
			conn, e := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		return nil, last
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("metadata redirects forbidden") }}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, id, nil)
	if e != nil {
		return Client{}, ErrInvalid
	}
	req.Header.Set("Accept", "application/json")
	res, e := client.Do(req)
	if e != nil {
		return Client{}, ErrInvalid
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(strings.ToLower(res.Header.Get("Content-Type")), "application/json") {
		return Client{}, ErrInvalid
	}
	body, e := io.ReadAll(io.LimitReader(res.Body, 32769))
	if e != nil || len(body) > 32768 {
		return Client{}, ErrInvalid
	}
	var c Client
	if json.Unmarshal(body, &c) != nil || c.ID != id || validateClient(c) != nil {
		return Client{}, ErrInvalid
	}
	entry := cachedClient{c, time.Now().Add(5 * time.Minute).Unix()}
	if r.ID != "" {
		_ = s.save(ctx, r, entry)
	} else {
		_ = s.store.Create(ctx, document("client:"+id, "client", 0, entry))
	}
	return c, nil
}
func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	for _, p := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/23", "64:ff9b::/96"} {
		if netip.MustParsePrefix(p).Contains(ip) {
			return false
		}
	}
	return true
}
func (s *Service) Register(ctx context.Context, c Client) (Client, error) {
	if !s.cfg.EnableRegistration || validateClient(c) != nil {
		return Client{}, ErrInvalid
	}
	if e := s.reserveRegistration(ctx); e != nil {
		return Client{}, e
	}
	var e error
	c.ID = "studio_" + secret()
	c.TokenEndpointAuthMethod = "none"
	c.GrantTypes = []string{"authorization_code", "refresh_token"}
	c.ResponseTypes = []string{"code"}
	if e = s.store.Create(ctx, document("registration:"+c.ID, "registration", 0, c)); e != nil {
		return Client{}, e
	}
	if e = s.store.Create(ctx, document("client:"+c.ID, "client", 0, cachedClient{Client: c})); e != nil {
		return Client{}, e
	}
	return c, nil
}

// A durable CAS counter bounds registration across processes and restarts.
func (s *Service) reserveRegistration(ctx context.Context) error {
	type limit struct {
		Count int
		Next  int64
	}
	for i := 0; i < 12; i++ {
		r, e := s.store.Get(ctx, "registration-limit")
		if errors.Is(e, ErrNotFound) {
			e = s.store.Create(ctx, document("registration-limit", "limit", 0, limit{}))
			if e == nil || errors.Is(e, ErrConflict) {
				continue
			}
			return e
		}
		if e != nil {
			return e
		}
		var v limit
		if json.Unmarshal(r.Payload, &v) != nil {
			return ErrInvalid
		}
		if v.Count >= 100 || v.Next > time.Now().Unix() {
			return ErrInvalid
		}
		v.Count++
		v.Next = time.Now().Add(time.Minute).Unix()
		e = s.save(ctx, r, v)
		if errors.Is(e, ErrConflict) {
			continue
		}
		return e
	}
	return ErrConflict
}
