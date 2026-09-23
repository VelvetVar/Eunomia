package eunomia

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type ScanRange struct {
	CIDR string
	Name string
}
type Found struct {
	Host      string
	Port      int
	Confirmed bool
	Banner    string
}
type ScanUpdate struct {
	Generation     int
	Checked, Total int
	Found          *Found
	Done           bool
	Err            error
}

func SubnetHosts(input string) (string, []string, error) {
	input = strings.TrimSpace(input)
	if !strings.Contains(input, "/") {
		input += "/32"
	}
	p, err := netip.ParsePrefix(input)
	if err != nil || !p.Addr().Is4() || p.Bits() < 24 || p.Bits() > 32 {
		return "", nil, errors.New("enter an IPv4 subnet from /24 to /32, or one IPv4 address")
	}
	p = p.Masked()
	a := p.Addr().As4()
	base := binary.BigEndian.Uint32(a[:])
	size := uint32(1) << uint(32-p.Bits())
	start, end := uint32(0), size
	if p.Bits() <= 30 {
		start = 1
		end = size - 1
	}
	hosts := []string{}
	for i := start; i < end; i++ {
		var bytes [4]byte
		binary.BigEndian.PutUint32(bytes[:], base+i)
		hosts = append(hosts, netip.AddrFrom4(bytes).String())
	}
	return p.String(), hosts, nil
}
func LocalRanges() []ScanRange {
	result := []ScanRange{}
	seen := map[string]bool{}
	interfaces, _ := net.Interfaces()
	for _, network := range interfaces {
		if network.Flags&net.FlagLoopback != 0 || network.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, _ := network.Addrs()
		for _, address := range addresses {
			p, err := netip.ParsePrefix(address.String())
			if err != nil || !p.Addr().Is4() || !p.Addr().IsPrivate() {
				continue
			}
			bits := p.Bits()
			if bits < 24 {
				bits = 24
			}
			cidr := netip.PrefixFrom(p.Addr(), bits).Masked().String()
			if !seen[cidr] {
				result = append(result, ScanRange{cidr, safe(network.Name)})
				seen[cidr] = true
			}
		}
	}
	return result
}
func ProbeSSH(ctx context.Context, host string, port int) *Found {
	deadline := time.Now().Add(900 * time.Millisecond)
	probeCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	var dialer net.Dialer
	conn, err := dialer.DialContext(probeCtx, "tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		return nil
	}
	defer conn.Close()
	conn.SetDeadline(deadline)
	stop := context.AfterFunc(probeCtx, func() { conn.Close() })
	defer stop()
	result := &Found{Host: host, Port: port}
	reader := bufio.NewReaderSize(conn, 512)
	read := 0
	for read < 512 {
		line, err := reader.ReadSlice('\n')
		read += len(line)
		if len(line) > 512 {
			line = line[:512]
		}
		text := strings.TrimSpace(safe(string(line)))
		if strings.HasPrefix(text, "SSH-") {
			result.Confirmed = true
			result.Banner = text
			break
		}
		if result.Banner == "" {
			result.Banner = text
		}
		if err != nil {
			break
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	if len(result.Banner) > 100 {
		result.Banner = result.Banner[:100]
	}
	return result
}
func Scan(ctx context.Context, cidr string, generation int, probe func(context.Context, string, int) *Found, emit func(ScanUpdate)) {
	_, hosts, err := SubnetHosts(cidr)
	if err != nil {
		emit(ScanUpdate{Generation: generation, Done: true, Err: err})
		return
	}
	if probe == nil {
		probe = ProbeSSH
	}
	jobs := make(chan string)
	results := make(chan *Found, 24)
	var wg sync.WaitGroup
	for i := 0; i < min(24, len(hosts)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for host := range jobs {
				if ctx.Err() != nil {
					return
				}
				found := probe(ctx, host, 22)
				select {
				case results <- found:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, host := range hosts {
			select {
			case jobs <- host:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
	checked := 0
	for found := range results {
		checked++
		if ctx.Err() == nil {
			emit(ScanUpdate{Generation: generation, Checked: checked, Total: len(hosts), Found: found})
		}
	}
	if ctx.Err() == nil {
		emit(ScanUpdate{Generation: generation, Checked: checked, Total: len(hosts), Done: true})
	}
}
func SortFound(found []Found) {
	sort.Slice(found, func(i, j int) bool {
		a, _ := netip.ParseAddr(found[i].Host)
		b, _ := netip.ParseAddr(found[j].Host)
		return a.Less(b)
	})
}

type Reachability struct {
	ID, Host, Status, Latency string
	CheckedAt                 time.Time
}

var latencyPattern = regexp.MustCompile(`(?i)time[=<]\s*([0-9.,]+\s*ms)`)

func PingArgs(host string, platform string) []string {
	switch platform {
	case "windows":
		return []string{"-n", "1", "-w", "2000", host}
	case "darwin":
		return []string{"-n", "-c", "1", host}
	default:
		return []string{"-n", "-c", "1", "-W", "2", host}
	}
}
func Ping(ctx context.Context, d Device) Reachability {
	r := Reachability{ID: d.ID, Host: d.Host, Status: "no reply", CheckedAt: time.Now()}
	tool := "ping"
	if runtime.GOOS == "darwin" && strings.Contains(d.Host, ":") {
		tool = "ping6"
	}
	file, err := Executable(tool)
	if err != nil {
		r.Status = "ping N/A"
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, file, PingArgs(d.Host, runtime.GOOS)...)
	cmd.WaitDelay = time.Second
	quietCommand(cmd)
	bytes, err := cmd.CombinedOutput()
	text := strings.ToLower(string(bytes))
	if err == nil && !strings.Contains(text, "unreachable") && !strings.Contains(text, "100% packet loss") && !strings.Contains(text, "100% loss") {
		r.Status = "reachable"
		if matches := latencyPattern.FindStringSubmatch(string(bytes)); len(matches) > 1 {
			r.Latency = matches[1]
		}
	}
	return r
}
func PingDevices(ctx context.Context, devices []Device, emit func(Reachability)) {
	jobs := make(chan Device)
	var wg sync.WaitGroup
	for i := 0; i < min(4, len(devices)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range jobs {
				if ctx.Err() != nil {
					return
				}
				r := Ping(ctx, d)
				if ctx.Err() == nil {
					emit(r)
				}
			}
		}()
	}
	for _, d := range devices {
		select {
		case jobs <- d:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		}
	}
	close(jobs)
	wg.Wait()
}
