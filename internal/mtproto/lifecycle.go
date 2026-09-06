package mtproto

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof" // runtime profiling when PprofAddr is configured
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// Lifecycle manages the lifecycle of the MTProto proxy, supporting atomic
// reloads without ever running two proxy instances in parallel.
type Lifecycle struct {
	mu           sync.Mutex
	cfg          Config
	metrics      *Metrics
	stream       *atomicEventStream
	currentProxy *Proxy
	realListener net.Listener

	curChanListener *channelListener
	curChanMu       sync.Mutex

	serving     atomic.Bool
	closed      atomic.Bool
	stopAccept  chan struct{}
	acceptWg    sync.WaitGroup
	pprofServer *http.Server
}

// NewLifecycle creates a new Lifecycle supervisor.
func NewLifecycle(cfg Config) (*Lifecycle, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	m := NewMetrics()
	return &Lifecycle{
		cfg:        cfg,
		metrics:    m,
		stream:     &atomicEventStream{metrics: m},
		stopAccept: make(chan struct{}),
	}, nil
}

// Metrics returns the active metrics tracker.
func (l *Lifecycle) Metrics() *Metrics {
	return l.metrics
}

// CurrentConfig returns the active configuration.
func (l *Lifecycle) CurrentConfig() Config {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cfg
}

// IsRunning reports whether the lifecycle is currently serving traffic.
func (l *Lifecycle) IsRunning() bool {
	return l.serving.Load()
}

// Snapshot returns a point-in-time copy of metrics including the running state.
func (l *Lifecycle) Snapshot() Snapshot {
	snap := l.metrics.Snapshot()
	snap.Running = l.serving.Load()
	return snap
}

// Run starts the MTProto proxy on the configured bind address and blocks until ctx is canceled.
func (l *Lifecycle) Run(ctx context.Context) error {
	l.mu.Lock()
	if l.serving.Load() {
		l.mu.Unlock()
		return errors.New("lifecycle already running")
	}

	ln, err := net.Listen("tcp", l.cfg.BindAddr)
	if err != nil {
		l.mu.Unlock()
		return fmt.Errorf("bind %s: %w", l.cfg.BindAddr, err)
	}
	l.realListener = ln

	// Start optional pprof server
	l.startPprofServer()

	// Create initial proxy instance
	proxy, err := NewProxy(l.cfg, l.stream)
	if err != nil {
		_ = ln.Close()
		l.mu.Unlock()
		return fmt.Errorf("create initial proxy: %w", err)
	}
	l.currentProxy = proxy
	l.serving.Store(true)

	// Create initial channel listener and start proxy serving
	chanLn := newChannelListener(ln.Addr())
	l.curChanListener = chanLn
	proxyDone := make(chan error, 1)
	go func() {
		proxyDone <- proxy.Serve(chanLn)
	}()
	l.mu.Unlock()

	slog.Info("mtproto proxy started", "addr", l.cfg.BindAddr, "max_conns", l.cfg.MaxConns)

	// Start accepting connections from real TCP listener and dispatching to channel listener
	l.acceptWg.Add(1)
	go l.acceptLoop()

	// Wait for context cancellation or explicit shutdown
	select {
	case <-ctx.Done():
		slog.Info("mtproto stopping on context cancellation")
	case <-l.stopAccept:
		slog.Info("mtproto stopped")
	}

	return l.Shutdown()
}

// acceptLoop accepts from real TCP socket and sends to current channelListener.
func (l *Lifecycle) acceptLoop() {
	defer l.acceptWg.Done()
	for {
		conn, err := l.realListener.Accept()
		if err != nil {
			if l.closed.Load() {
				return
			}
			select {
			case <-l.stopAccept:
				return
			default:
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}

		l.curChanMu.Lock()
		chanLn := l.curChanListener
		l.curChanMu.Unlock()

		if chanLn != nil {
			if !chanLn.dispatch(conn) {
				_ = conn.Close()
			}
		} else {
			_ = conn.Close()
		}
	}
}

// Reload applies a new configuration atomically without running two proxy instances in parallel.
func (l *Lifecycle) Reload(newCfg Config) error {
	if err := newCfg.Validate(); err != nil {
		return fmt.Errorf("validate new config: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.serving.Load() || l.closed.Load() {
		return errors.New("cannot reload: proxy not serving")
	}

	// If bind address changed, we need to reopen real listener
	needRebind := newCfg.BindAddr != l.cfg.BindAddr

	slog.Info("mtproto atomic reload initiated", "old_addr", l.cfg.BindAddr, "new_addr", newCfg.BindAddr)

	// 1. Unlink current channel listener so incoming connections are held in TCP backlog
	l.curChanMu.Lock()
	oldChanLn := l.curChanListener
	l.curChanListener = nil
	l.curChanMu.Unlock()

	// 2. Shut down old proxy cleanly
	if oldChanLn != nil {
		_ = oldChanLn.Close()
	}
	if l.currentProxy != nil {
		l.currentProxy.Shutdown()
		l.currentProxy = nil
	}

	// 3. Immediately trigger GC and return freed memory pages to the OS (< 96 MB RAM constraint)
	runtime.GC()
	debug.FreeOSMemory()

	// 4. If rebind needed, replace real listener
	if needRebind {
		_ = l.realListener.Close()
		newRealLn, err := net.Listen("tcp", newCfg.BindAddr)
		if err != nil {
			return fmt.Errorf("rebind new addr %s: %w", newCfg.BindAddr, err)
		}
		l.realListener = newRealLn
	}

	// 5. Instantiate new proxy instance (guaranteed to be the ONLY instance in memory)
	newProxy, err := NewProxy(newCfg, l.stream)
	if err != nil {
		return fmt.Errorf("create reloaded proxy: %w", err)
	}
	l.currentProxy = newProxy
	l.cfg = newCfg

	// 6. Create new channel listener and start serving
	newChanLn := newChannelListener(l.realListener.Addr())
	l.curChanMu.Lock()
	l.curChanListener = newChanLn
	l.curChanMu.Unlock()

	go func() {
		if err := newProxy.Serve(newChanLn); err != nil && !errors.Is(err, net.ErrClosed) {
			slog.Error("reloaded mtproto serve error", "err", err)
		}
	}()

	slog.Info("mtproto atomic reload completed successfully")
	return nil
}

// Shutdown gracefully stops the proxy and listeners.
// Close gracefully shuts down the proxy lifecycle.
func (l *Lifecycle) Close() error {
	return l.Shutdown()
}

func (l *Lifecycle) Shutdown() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed.Swap(true) {
		return nil
	}
	l.serving.Store(false)

	close(l.stopAccept)
	if l.realListener != nil {
		_ = l.realListener.Close()
	}

	l.curChanMu.Lock()
	if l.curChanListener != nil {
		_ = l.curChanListener.Close()
	}
	l.curChanMu.Unlock()

	if l.currentProxy != nil {
		l.currentProxy.Shutdown()
	}

	l.acceptWg.Wait()

	if l.pprofServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = l.pprofServer.Shutdown(ctx)
	}

	runtime.GC()
	debug.FreeOSMemory()

	slog.Info("mtproto proxy stopped gracefully")
	return nil
}

func (l *Lifecycle) startPprofServer() {
	if l.cfg.PprofAddr == "" {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)
	l.pprofServer = &http.Server{
		Addr:    l.cfg.PprofAddr,
		Handler: mux,
	}
	go func() {
		slog.Info("mtproto pprof server started", "addr", l.cfg.PprofAddr)
		if err := l.pprofServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Warn("mtproto pprof server error", "err", err)
		}
	}()
}

// channelListener implements net.Listener by delivering accepted net.Conn via a Go channel.
type channelListener struct {
	conns  chan net.Conn
	done   chan struct{}
	closed atomic.Bool
	addr   net.Addr
}

func newChannelListener(addr net.Addr) *channelListener {
	return &channelListener{
		conns: make(chan net.Conn, 64),
		done:  make(chan struct{}),
		addr:  addr,
	}
}

func (c *channelListener) Accept() (net.Conn, error) {
	select {
	case conn, ok := <-c.conns:
		if !ok {
			return nil, net.ErrClosed
		}
		return conn, nil
	case <-c.done:
		return nil, net.ErrClosed
	}
}

func (c *channelListener) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	close(c.done)
	// Drain any unhandled connections
	for {
		select {
		case conn := <-c.conns:
			_ = conn.Close()
		default:
			return nil
		}
	}
}

func (c *channelListener) Addr() net.Addr {
	return c.addr
}

func (c *channelListener) dispatch(conn net.Conn) bool {
	if c.closed.Load() {
		return false
	}
	select {
	case c.conns <- conn:
		return true
	case <-c.done:
		return false
	}
}
