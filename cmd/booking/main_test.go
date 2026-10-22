package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// binaryPath is the booking binary built once by TestMain from this package's sources: the
// end-to-end tests below run the real executable, not an in-process handler.
var binaryPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "booking-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binaryPath = filepath.Join(dir, "booking")
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build booking binary:", err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// service is one running booking process.
type service struct {
	url    string
	cmd    *exec.Cmd
	stderr *lockedBuffer
	exited chan struct{}
	err    error // valid once exited is closed
}

// startService runs the binary against dbPath on a free loopback port and waits until it answers
// HTTP. A free port can be taken between choosing and binding it; the process then exits at once
// and the start is retried on another port.
func startService(t *testing.T, dbPath string) *service {
	t.Helper()
	for attempt := 0; attempt < 3; attempt++ {
		addr := freeAddr(t)
		s := &service{url: "http://" + addr, stderr: &lockedBuffer{}, exited: make(chan struct{})}
		s.cmd = exec.Command(binaryPath)
		s.cmd.Env = append(os.Environ(), "BOOKING_DB_PATH="+dbPath, "BOOKING_ADDR="+addr)
		s.cmd.Stderr = s.stderr
		if err := s.cmd.Start(); err != nil {
			t.Fatalf("start binary: %v", err)
		}
		go func() { s.err = s.cmd.Wait(); close(s.exited) }()
		if s.waitReady() {
			t.Cleanup(func() {
				_ = s.cmd.Process.Kill()
				<-s.exited
			})
			return s
		}
		select {
		case <-s.exited:
			continue // most likely lost the port race
		default:
			_ = s.cmd.Process.Kill()
			<-s.exited
			t.Fatalf("service did not become ready; stderr: %s", s.stderr)
		}
	}
	t.Fatal("service did not start after 3 attempts")
	return nil
}

// lockedBuffer collects the child's stderr; os/exec writes it from its own goroutine while the
// test may read it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func (s *service) waitReady() bool {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-s.exited:
			return false
		default:
		}
		resp, err := http.Get(s.url + "/no-such-path")
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func postJSON(client *http.Client, url string, body any) (int, []byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

// futureMondayNine is a Monday 09:00 UTC at least a week after the real current time: the binary
// runs with the system clock, and the slot must be in the future relative to it.
func futureMondayNine() time.Time {
	d := time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour)
	for d.Weekday() != time.Monday {
		d = d.AddDate(0, 0, 1)
	}
	return d.Add(9 * time.Hour)
}

func createResource(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()
	var hours []map[string]string
	for _, d := range []string{"monday", "tuesday", "wednesday", "thursday", "friday"} {
		hours = append(hours, map[string]string{"weekday": d, "start": "09:00", "end": "17:00"})
	}
	status, body, err := postJSON(client, baseURL+"/resources", map[string]any{"name": "Dr. Lee", "weeklyHours": hours})
	if err != nil || status != http.StatusCreated {
		t.Fatalf("create resource: status %d, err %v, body %s", status, err, body)
	}
	var r struct{ ID string }
	if err := json.Unmarshal(body, &r); err != nil || r.ID == "" {
		t.Fatalf("create resource: no id in %s (%v)", body, err)
	}
	return r.ID
}

// AC-019 (NFR-001's checkpoint, INV-001, ADR-0012): the real binary on a fresh SQLite file, 50
// simultaneous requests with distinct keys for one free future slot: exactly one 201, 49 × 409
// slot_taken, and nothing else — in particular no 500 from a lock or busy error.
func TestBookSlot_50ConcurrentRequestsExactlyOneWins(t *testing.T) {
	const writers = 50
	dbPath := filepath.Join(t.TempDir(), "booking.db")
	svc := startService(t, dbPath)
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{MaxIdleConnsPerHost: writers, DisableKeepAlives: false},
	}
	resourceID := createResource(t, client, svc.url)
	slot := futureMondayNine()

	type result struct {
		status int
		body   []byte
		err    error
	}
	results := make([]result, writers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			<-start
			status, body, err := postJSON(client, svc.url+"/bookings", map[string]any{
				"resourceId":     resourceID,
				"slotStart":      slot.Format(time.RFC3339),
				"customerName":   fmt.Sprintf("Customer %02d", i),
				"idempotencyKey": fmt.Sprintf("key-%02d", i),
			})
			results[i] = result{status, body, err}
		})
	}
	close(start)
	wg.Wait()

	created, taken := 0, 0
	for i, r := range results {
		switch {
		case r.err != nil:
			t.Errorf("request %d: transport error %v", i, r.err)
		case r.status == http.StatusCreated:
			created++
		case r.status == http.StatusConflict:
			var e struct{ Code string }
			if err := json.Unmarshal(r.body, &e); err != nil || e.Code != "slot_taken" {
				t.Errorf("request %d: 409 with body %s, want code slot_taken", i, r.body)
			}
			taken++
		default:
			t.Errorf("request %d: status %d body %s; want 201 or 409 slot_taken (no lock/busy error)", i, r.status, r.body)
		}
	}
	if created != 1 || taken != writers-1 {
		t.Fatalf("created %d, slot_taken %d; want 1 and %d", created, taken, writers-1)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var active int
	if err := db.QueryRow(
		"SELECT count(*) FROM booking WHERE resource_id = ? AND slot_start = ? AND status = 'active'",
		resourceID, slot.Unix()).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active bookings for the slot = %d, want 1", active)
	}
	if strings.TrimSpace(svc.stderr.String()) != "" {
		t.Errorf("unexpected stderr output: %s", svc.stderr)
	}
}

// The bootstrap stops on SIGTERM: in-flight work drains within shutdownTimeout and the process
// exits with status 0.
func TestRun_ExitsCleanlyOnSIGTERM(t *testing.T) {
	svc := startService(t, filepath.Join(t.TempDir(), "booking.db"))
	if err := svc.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-svc.exited:
	case <-time.After(shutdownTimeout + 5*time.Second):
		t.Fatal("process did not exit after SIGTERM")
	}
	if svc.err != nil {
		t.Fatalf("exit after SIGTERM: %v; stderr: %s", svc.err, svc.stderr)
	}
}

// Without a database path the binary refuses to start: exit status 1 and one line on stderr.
func TestRun_MissingDatabasePathFailsToStart(t *testing.T) {
	cmd := exec.Command(binaryPath)
	cmd.Env = []string{"BOOKING_ADDR=127.0.0.1:0"}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("exit: %v, want status 1", err)
	}
	if !strings.Contains(stderr.String(), "BOOKING_DB_PATH") {
		t.Fatalf("stderr %q does not name BOOKING_DB_PATH", stderr.String())
	}
}

func TestLoadConfig(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

	if _, err := loadConfig(env(nil)); err == nil {
		t.Error("missing BOOKING_DB_PATH accepted")
	}
	c, err := loadConfig(env(map[string]string{"BOOKING_DB_PATH": "/data/booking.db"}))
	if err != nil || c.dbPath != "/data/booking.db" || c.addr != defaultAddr {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	c, err = loadConfig(env(map[string]string{"BOOKING_DB_PATH": "x.db", "BOOKING_ADDR": "127.0.0.1:9000"}))
	if err != nil || c.addr != "127.0.0.1:9000" {
		t.Errorf("explicit addr: %+v, %v", c, err)
	}
}
