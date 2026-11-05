package mcp

import (
	"testing"
	"time"

	"github.com/amjadjibon/memsh/pkg/shell"
)

func TestPoolEnforcesSessionCap(t *testing.T) {
	p := NewPool(2, time.Second, shell.WithWASMEnabled(false))
	defer p.Close()

	if p.GetServer(nil) == nil || p.GetServer(nil) == nil {
		t.Fatal("expected the first two servers to be created")
	}
	if p.GetServer(nil) != nil {
		t.Fatal("third server created past the cap of 2")
	}
	if p.Len() != 2 {
		t.Fatalf("Len = %d, want 2", p.Len())
	}
}

func TestPoolReapsSessionlessServersAfterGrace(t *testing.T) {
	now := time.Now()
	p := NewPool(1, time.Second, shell.WithWASMEnabled(false))
	p.now = func() time.Time { return now }
	defer p.Close()

	if p.GetServer(nil) == nil {
		t.Fatal("expected a server")
	}
	// Within the grace period a server without a bound session is kept,
	// because the SDK binds the session only after getServer returns.
	p.Sweep()
	if p.Len() != 1 {
		t.Fatalf("server reaped inside grace period; Len = %d", p.Len())
	}

	now = now.Add(sessionGrace + time.Second)
	if p.GetServer(nil) == nil {
		t.Fatal("expected the stale server to be reaped and a new one created under the cap")
	}
	if p.Len() != 1 {
		t.Fatalf("Len = %d, want 1 after reaping", p.Len())
	}
}
