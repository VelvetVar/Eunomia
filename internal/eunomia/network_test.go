package eunomia

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubnetBounds(t *testing.T) {
	cidr, hosts, err := SubnetHosts("192.168.1.90/24")
	if err != nil || cidr != "192.168.1.0/24" || len(hosts) != 254 || hosts[0] != "192.168.1.1" || hosts[253] != "192.168.1.254" {
		t.Fatal(cidr, len(hosts), err)
	}
	_, hosts, err = SubnetHosts("10.0.0.1/31")
	if err != nil || strings.Join(hosts, ",") != "10.0.0.0,10.0.0.1" {
		t.Fatal(hosts, err)
	}
	for _, bad := range []string{"10.0.0.0/8", "::1", "example.com", "192.168.9.999/24"} {
		if _, _, err := SubnetHosts(bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}
func TestScanConcurrencyAndCancellation(t *testing.T) {
	var active, peak, calls atomic.Int32
	probe := func(ctx context.Context, host string, port int) *Found {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		calls.Add(1)
		select {
		case <-time.After(time.Millisecond):
		case <-ctx.Done():
			return nil
		}
		return &Found{Host: host, Port: port, Confirmed: true}
	}
	var last ScanUpdate
	Scan(context.Background(), "192.168.1.0/24", 7, probe, func(u ScanUpdate) { last = u })
	if peak.Load() > 24 || peak.Load() < 2 || calls.Load() != 254 || !last.Done || last.Checked != 254 || last.Generation != 7 {
		t.Fatal(peak.Load(), calls.Load(), last)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls.Store(0)
	Scan(ctx, "10.0.0.0/24", 8, probe, func(u ScanUpdate) { t.Fatal("cancelled scan published", u) })
	if calls.Load() != 0 {
		t.Fatal("cancelled scan dialed")
	}
}
func TestProbeSSHLocalOnlyAndNoClientPayload(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan int, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprint(conn, "Welcome\r\nSSH-2.0-EunomiaFixture\r\n")
		conn.SetReadDeadline(time.Now().Add(time.Second))
		b := make([]byte, 1)
		n, _ := conn.Read(b)
		received <- n
	}()
	result := ProbeSSH(context.Background(), "127.0.0.1", listener.Addr().(*net.TCPAddr).Port)
	if result == nil || !result.Confirmed || result.Banner != "SSH-2.0-EunomiaFixture" {
		t.Fatal(result)
	}
	if <-received != 0 {
		t.Fatal("sent payload")
	}
}
