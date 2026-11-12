package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(body)
}

// start runs the server with args on a free port; stop cancels it and
// returns the server logs once run has returned.
func start(t *testing.T, args ...string) (base string, stop func() string) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	logs := &syncBuffer{}
	addrc := make(chan string, 1)
	errc := make(chan error, 1)
	db := filepath.Join(t.TempDir(), "wheels.db")
	args = append([]string{"-addr", "127.0.0.1:0", "-db", db, "-log-level", "debug"}, args...)
	go func() { errc <- run(ctx, args, logs, func(a string) { addrc <- a }) }()

	select {
	case a := <-addrc:
		base = "http://" + a
	case err := <-errc:
		cancel()
		t.Fatalf("run exited early: %v", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("server did not start")
	}
	return base, func() string {
		t.Helper()
		cancel()
		select {
		case err := <-errc:
			if err != nil {
				t.Fatalf("run: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("server did not shut down")
		}
		return logs.String()
	}
}

func TestRunServesAndShutsDown(t *testing.T) {
	base, stop := start(t)
	if code, body := get(t, base+"/healthz"); code != http.StatusOK || !strings.Contains(body, "ok") {
		t.Errorf("healthz = %d %q", code, body)
	}
	if code, body := get(t, base+"/"); code != http.StatusOK || !strings.Contains(body, "Wheel Tuner") {
		t.Errorf("index = %d", code)
	}
	if code, _ := get(t, base+"/js/viz/radar.js"); code != http.StatusOK {
		t.Errorf("radar.js = %d", code)
	}
	if out := stop(); !strings.Contains(out, "shutting down") || !strings.Contains(out, "http request") {
		t.Errorf("logs missing lifecycle/debug entries:\n%s", out)
	}
}

func TestRunAPIOnly(t *testing.T) {
	base, stop := start(t, "-web=false")
	defer stop()
	if code, _ := get(t, base+"/healthz"); code != http.StatusOK {
		t.Errorf("healthz = %d", code)
	}
	if code, body := get(t, base+"/api/tensiometers"); code != http.StatusOK || !strings.Contains(body, "ztto-tc-02") {
		t.Errorf("tensiometers = %d", code)
	}
	if code, _ := get(t, base+"/"); code != http.StatusNotFound {
		t.Errorf("index without the web UI = %d, want 404", code)
	}
}
func TestRunErrors(t *testing.T) {
	busy, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()

	dir := t.TempDir()
	tests := []struct {
		name string
		args []string
		want error
	}{
		{"help", []string{"-h"}, flag.ErrHelp},
		{"unknown flag", []string{"-nope"}, nil},
		{"bad log level", []string{"-log-level", "loud"}, nil},
		{"positional args", []string{"extra"}, nil},
		{"unopenable db", []string{"-db", filepath.Join(dir, "missing", "x.db")}, nil},
		{"address in use", []string{"-addr", busy.Addr().String(), "-db", filepath.Join(dir, "ok.db")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(t.Context(), tt.args, io.Discard, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
