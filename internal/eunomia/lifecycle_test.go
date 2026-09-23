package eunomia

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
