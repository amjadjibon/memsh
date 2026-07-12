package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/amjadjibon/memsh/internal/server"
	"github.com/amjadjibon/memsh/internal/session"
	"github.com/amjadjibon/memsh/pkg/shell"
)

func newTestServer(t *testing.T) (*server.Handler, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	store := session.New(ctx, 30*time.Minute, 0)
	baseOpts := []shell.Option{shell.WithWASMEnabled(false), shell.WithInheritEnv(false)}
	h := server.NewHandler(store, baseOpts, 30*time.Second, session.Limits{})
	return h, cancel
}

func mux(h *server.Handler) *http.ServeMux {
	m := http.NewServeMux()
	h.RegisterRoutes(m)
	return m
}

func TestHealthEndpoint(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
}

func TestRunEndpoint(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	body := `{"script":"echo hello"}`
	resp, err := http.Post(srv.URL+"/run", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, _ := result["output"].(string)
	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want to contain 'hello'", out)
	}
}

func TestRunEndpointEmptyScript(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/run", "application/json", strings.NewReader(`{"script":""}`))
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestRunEndpointInvalidJSON(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/run", "application/json", strings.NewReader(`not json`))
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestRunEndpointWithSession(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	// First request creates the session.
	req1, _ := http.NewRequest("POST", srv.URL+"/run", strings.NewReader(`{"script":"mkdir /mydir"}`))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Session-ID", "aaaaaaaaaaaaaaaa")
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	resp1.Body.Close()

	if resp1.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp1.StatusCode)
	}

	// Second request with same session should see /mydir.
	req2, _ := http.NewRequest("POST", srv.URL+"/run", strings.NewReader(`{"script":"ls /"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Session-ID", "aaaaaaaaaaaaaaaa")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp2.Body.Close()

	var result map[string]any
	json.NewDecoder(resp2.Body).Decode(&result)
	out, _ := result["output"].(string)
	if !strings.Contains(out, "mydir") {
		t.Errorf("output = %q, want to contain 'mydir'", out)
	}
}

func TestSessionsEndpoint(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sessions")
	if err != nil {
		t.Fatalf("GET /sessions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestSessionDeleteEndpoint(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	// Create a session by running a command.
	req, _ := http.NewRequest("POST", srv.URL+"/run", strings.NewReader(`{"script":"echo hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", "bbbbbbbbbbbbbbbb")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	// Delete the session.
	delReq, _ := http.NewRequest("DELETE", srv.URL+"/session/bbbbbbbbbbbbbbbb", nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE /session: %v", err)
	}
	defer delResp.Body.Close()

	if delResp.StatusCode != 204 {
		t.Errorf("status = %d, want 204", delResp.StatusCode)
	}
}

func TestSnapshotGetEndpoint(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	// Create session with some files.
	req, _ := http.NewRequest("POST", srv.URL+"/run", strings.NewReader(`{"script":"echo data > /file.txt"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", "cccccccccccccccc")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	// Get snapshot.
	getReq, _ := http.NewRequest("GET", srv.URL+"/session/cccccccccccccccc/snapshot", nil)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("GET snapshot: %v", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", getResp.StatusCode)
	}

	data, _ := io.ReadAll(getResp.Body)
	if len(data) == 0 {
		t.Error("snapshot body is empty")
	}
}

func TestSnapshotPostEndpoint(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	// Create session, write a file, export snapshot.
	req1, _ := http.NewRequest("POST", srv.URL+"/run", strings.NewReader(`{"script":"echo imported > /test.txt"}`))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Session-ID", "dddddddddddddddd")
	resp1, _ := http.DefaultClient.Do(req1)
	resp1.Body.Close()

	getReq, _ := http.NewRequest("GET", srv.URL+"/session/dddddddddddddddd/snapshot", nil)
	getResp, _ := http.DefaultClient.Do(getReq)
	snapData, _ := io.ReadAll(getResp.Body)
	getResp.Body.Close()

	// Import into a new session.
	postReq, _ := http.NewRequest("POST", srv.URL+"/session/new/snapshot", bytes.NewReader(snapData))
	postReq.Header.Set("Content-Type", "application/json")
	postResp, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatalf("POST snapshot: %v", err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", postResp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(postResp.Body).Decode(&result)
	if _, ok := result["session_id"]; !ok {
		t.Error("response missing session_id")
	}
}

func TestCompleteEndpoint(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	body := `{"input":"ec","cursor":2}`
	resp, err := http.Post(srv.URL+"/complete", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /complete: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	completions, _ := result["completions"].([]any)
	if len(completions) == 0 {
		t.Error("expected completions for 'ec'")
	}
}

func TestIndexEndpoint(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestIndexNotFound(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/nonexistent")
	if err != nil {
		t.Fatalf("GET /nonexistent: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestInvalidSessionIDRejected(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/run", strings.NewReader(`{"script":"echo hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", "weak")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSessionIDNewMints(t *testing.T) {
	h, cancel := newTestServer(t)
	defer cancel()
	srv := httptest.NewServer(mux(h))
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/run", strings.NewReader(`{"script":"echo hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", "new")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /run: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	sid, _ := result["session_id"].(string)
	if len(sid) < 16 {
		t.Errorf("session_id = %q, want minted id", sid)
	}
}
