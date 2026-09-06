package mtproto

import (
	"fmt"
	"net"
	"sync"

	"github.com/9seconds/mtg/v2/antireplay"
	"github.com/9seconds/mtg/v2/ipblocklist"
	"github.com/9seconds/mtg/v2/logger"
	"github.com/9seconds/mtg/v2/mtglib"
	"github.com/9seconds/mtg/v2/network"
)

// Proxy wraps mtglib.Proxy with our memory-constrained configuration.
type Proxy struct {
	raw       *mtglib.Proxy
	mu        sync.Mutex
	listener  net.Listener
	started   bool
	serveDone chan struct{}
	closeOnce sync.Once
}

// NewProxy constructs an optimized mtglib.Proxy instance.
func NewProxy(cfg Config, stream mtglib.EventStream) (*Proxy, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	secret, err := ParseSecret(cfg.Secret)
	if err != nil {
		return nil, fmt.Errorf("parse secret: %w", err)
	}

	dialer, err := network.NewDefaultDialer(cfg.HandshakeTimeout, cfg.BufferSize)
	if err != nil {
		return nil, fmt.Errorf("create dialer: %w", err)
	}

	ntw, err := network.NewNetwork(dialer, "Mozilla/5.0", "1.1.1.1", cfg.HandshakeTimeout)
	if err != nil {
		return nil, fmt.Errorf("create network: %w", err)
	}

	var antiReplay mtglib.AntiReplayCache = antireplay.NewNoop()
	if cfg.AntiReplay {
		// Allocate a compact 1 MB stable bloom filter for replay protection
		antiReplay = antireplay.NewStableBloomFilter(1<<20, 0.001)
	}

	opts := mtglib.ProxyOpts{
		Secret:           secret,
		Network:          ntw,
		AntiReplayCache:  antiReplay,
		IPBlocklist:      ipblocklist.NewNoop(),
		IPAllowlist:      ipblocklist.NewNoop(),
		EventStream:      stream,
		Logger:           logger.NewNoopLogger(), // zero-alloc, disable verbose trace/debug logs
		Concurrency:      cfg.MaxConns,
		IdleTimeout:      cfg.IdleTimeout,
		HandshakeTimeout: cfg.HandshakeTimeout,
	}

	raw, err := mtglib.NewProxy(opts)
	if err != nil {
		return nil, fmt.Errorf("init mtglib proxy: %w", err)
	}

	return &Proxy{
		raw:       raw,
		serveDone: make(chan struct{}),
	}, nil
}

// Serve accepts incoming connections on the listener and delegates them to mtglib.
func (p *Proxy) Serve(l net.Listener) error {
	if p.raw == nil {
		return fmt.Errorf("proxy not initialized")
	}
	p.mu.Lock()
	p.listener = l
	p.started = true
	p.mu.Unlock()

	defer p.closeOnce.Do(func() { close(p.serveDone) })
	return p.raw.Serve(l)
}

// Shutdown gracefully terminates all active connections on this proxy instance.
func (p *Proxy) Shutdown() {
	if p.raw == nil {
		return
	}
	p.mu.Lock()
	started := p.started
	ln := p.listener
	p.mu.Unlock()

	if started {
		if ln != nil {
			_ = ln.Close()
		}
		<-p.serveDone
	}

	p.raw.Shutdown()
}
