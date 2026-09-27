package eunomia

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDuplicateLaunchShowsErrorForThreeSeconds(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("EUNOMIA_HOME", directory)
	stopped := make(chan struct{}, 1)
	control, err := StartControl(directory, func() { stopped <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	path := filepath.Join(directory, "running.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	start := time.Now()
	code := Main(nil, &out, &errOut)
	if code != 1 || out.Len() != 0 || errOut.String() != "Application is already running\n" {
		t.Fatal("unexpected duplicate-launch response", code, out.String(), errOut.String())
	}
	if time.Since(start) < 3*time.Second {
		t.Fatal("duplicate closed before the three-second message elapsed")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("duplicate changed the running instance record", err)
	}
	select {
	case <-stopped:
		t.Fatal("duplicate stopped the existing instance")
	default:
	}
}

func TestAuthenticatedLifecycle(t *testing.T) {
	directory := t.TempDir()
	stopped := make(chan struct{}, 1)
	control, err := StartControl(directory, func() { stopped <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	if other, err := StartControl(directory, func() {}); err == nil {
		other.Close()
		t.Fatal("duplicate accepted")
	}
	conn, err := net.Dial("tcp", control.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	json.NewEncoder(conn).Encode(controlRequest{"down", strings.Repeat("0", 64)})
	var reply controlReply
	json.NewDecoder(conn).Decode(&reply)
	conn.Close()
	if reply.OK {
		t.Fatal("unauthenticated request accepted")
	}
	select {
	case <-stopped:
		t.Fatal("stopped without auth")
	default:
	}
	ok, err := StopRunning(directory)
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("did not stop")
	}
	control.Close()
	if _, err := os.Stat(filepath.Join(directory, "running.json")); !os.IsNotExist(err) {
		t.Fatal("runtime record remains")
	}
	ok, err = StopRunning(directory)
	if ok || err != nil {
		t.Fatal(ok, err)
	}
}
func TestLifecyclePreservesLiveUnreachableState(t *testing.T) {
	directory := t.TempDir()
	state := runState{os.Getpid(), 1, strings.Repeat("0", 64)}
	bytes, _ := json.Marshal(state)
	os.WriteFile(filepath.Join(directory, "running.json"), bytes, 0600)
	if _, err := StopRunning(directory); err == nil {
		t.Fatal("unreachable live PID accepted")
	}
	if _, err := os.Stat(filepath.Join(directory, "running.json")); err != nil {
		t.Fatal("live record removed")
	}
}

func TestControlRejectsExcessConnectionsAndRecovers(t *testing.T) {
	directory := t.TempDir()
	stopped := make(chan struct{}, 1)
	control, err := StartControl(directory, func() { stopped <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	for range maxControlConnections {
		conn, err := net.DialTimeout("tcp4", control.listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
	}
	waitUntil(t, time.Second, func() bool { return len(control.slots) == maxControlConnections })
	extra, err := net.DialTimeout("tcp4", control.listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()
	extra.SetReadDeadline(time.Now().Add(time.Second))
	var reply [1]byte
	if _, err := extra.Read(reply[:]); err == nil {
		t.Fatal("excess connection was served")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("excess connection remained queued")
	}
	// Idle requests expire, restoring capacity for an authenticated command.
	waitUntil(t, 4*time.Second, func() bool { return len(control.slots) == 0 })
	if ok, err := StopRunning(directory); !ok || err != nil {
		t.Fatal("control did not recover after overload", ok, err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("authenticated stop was not delivered")
	}
}
