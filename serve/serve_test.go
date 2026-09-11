package serve

import (
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestRunReportsAnUnbindableAddress(t *testing.T) {
	// Hold the port so Run cannot have it. A bind failure has to surface as an
	// error rather than a process that sits there serving nothing.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	defer ln.Close()

	err = Run(ln.Addr().String(), http.NewServeMux())
	if err == nil {
		t.Fatal("Run returned nil for an address already in use")
	}
	if !strings.Contains(err.Error(), "listen and serve") {
		t.Errorf("error not wrapped with its origin: %v", err)
	}
}

func TestOptionsAccumulate(t *testing.T) {
	var cfg config
	ran := false
	WithLogAttrs("db", "/tmp/x")(&cfg)
	WithLogAttrs("mode", "wal")(&cfg)
	WithPreShutdown(func() { ran = true })(&cfg)

	if len(cfg.logAttrs) != 4 {
		t.Errorf("logAttrs = %v, want four elements from two calls", cfg.logAttrs)
	}
	if cfg.onShutdown == nil {
		t.Fatal("WithPreShutdown did not set the hook")
	}
	cfg.onShutdown()
	if !ran {
		t.Error("hook was not the function passed in")
	}
}
