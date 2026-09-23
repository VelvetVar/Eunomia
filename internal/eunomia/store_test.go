package eunomia

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureDevice() Device {
	return Device{Name: "Atlas NAS", Host: "192.168.1.20", Username: "admin", Port: 22, Description: "Storage"}
}
func TestStoreMigrationAndPersistence(t *testing.T) {
	s := Store{t.TempDir()}
	legacy := `{"version":1,"devices":[{"id":"legacy-id","name":"Legacy NAS","host":"2001:db8::1","username":"admin","port":2222,"description":"Existing Node profile","createdAt":"2026-01-01T00:00:00.000Z","updatedAt":"2026-01-01T00:00:00.000Z","password":"must-not-survive"}]}`
	if err := os.WriteFile(s.Path(), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	devices, err := s.Read()
	if err != nil || len(devices) != 1 {
		t.Fatalf("read: %v %+v", err, devices)
	}
	d := devices[0]
	d.Name = "Renamed"
	saved, err := s.Save(d, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID != "legacy-id" || saved.CreatedAt != d.CreatedAt {
		t.Fatal("identity/timestamp changed")
	}
	contents, _ := os.ReadFile(s.Path())
	if bytes.Contains(contents, []byte("password")) {
		t.Fatal("unknown credential field persisted")
	}
	added, err := s.Save(fixtureDevice(), "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := (Store{s.Directory}).Read()
	if err != nil || len(again) != 2 {
		t.Fatal(err, again)
	}
	if err = s.Remove(added.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(d, ""); err == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestStorePreservesCorruptionAndHonorsLock(t *testing.T) {
	s := Store{t.TempDir()}
	original := []byte(`{"version":9,"devices":[]}`)
	os.WriteFile(s.Path(), original, 0600)
	if _, err := s.Save(fixtureDevice(), ""); err == nil {
		t.Fatal("corrupt store overwritten")
	}
	after, _ := os.ReadFile(s.Path())
	if !bytes.Equal(original, after) {
		t.Fatal("corrupt file changed")
	}
	os.Remove(s.Path())
	os.WriteFile(s.Path()+".lock", nil, 0600)
	if _, err := s.Save(fixtureDevice(), ""); err == nil {
		t.Fatal("lock ignored")
	}
	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Fatal("wrote despite lock")
	}
}
func TestValidationAndArgumentSafety(t *testing.T) {
	for _, host := range []string{"192.168.0.1", "2001:db8::1", "atlas.local", "Atlas"} {
		d := fixtureDevice()
		d.Host = host
		if _, err := Validate(d); err != nil {
			t.Fatal(host, err)
		}
	}
	for _, host := range []string{"-oProxyCommand=bad", "192.168.0.999", "ssh://atlas", "evil;touch file", "atlas\n"} {
		d := fixtureDevice()
		d.Host = host
		if host == "atlas\n" {
			d.Host = "at\nlas"
		}
		if _, err := Validate(d); err == nil {
			t.Fatal("accepted", host)
		}
	}
	d := fixtureDevice()
	d.Port = 0
	if _, err := Validate(d); err == nil {
		t.Fatal("zero port")
	}
	d = fixtureDevice()
	d.Username = "admin;bad"
	if _, err := Validate(d); err == nil {
		t.Fatal("unsafe username")
	}
	d = fixtureDevice()
	d.Port = 2222
	args := SSHArgs(d)
	if strings.Join(args, " ") != "-p 2222 -l admin 192.168.1.20" {
		t.Fatal(args)
	}
}
func TestCLIWorkflow(t *testing.T) {
	t.Setenv("EUNOMIA_HOME", t.TempDir())
	run := func(args ...string) (int, string) {
		var out, errOut bytes.Buffer
		code := Main(args, &out, &errOut)
		return code, out.String() + errOut.String()
	}
	if code, out := run("add", "NAS with spaces", "--host", "127.0.0.1", "--user", "tester"); code != 0 {
		t.Fatal(out)
	}
	code, out := run("list", "--json")
	var devices []Device
	if code != 0 || json.Unmarshal([]byte(out), &devices) != nil || len(devices) != 1 {
		t.Fatal(out)
	}
	if code, out = run("edit", "NAS with spaces", "--port", "2222"); code != 0 {
		t.Fatal(out)
	}
	if code, out = run("connect", "NAS with spaces", "--dry-run"); code != 0 || !strings.Contains(out, "2222") {
		t.Fatal(out)
	}
	if code, _ = run("remove", "NAS with spaces"); code == 0 {
		t.Fatal("removal lacked confirmation")
	}
	if code, out = run("remove", "NAS with spaces", "--yes"); code != 0 {
		t.Fatal(out)
	}
	if code, _ = run("add", "x", "--password", "secret"); code == 0 {
		t.Fatal("password flag accepted")
	}
	if code, out = run("down"); code != 0 || !strings.Contains(out, "not running") {
		t.Fatal(out)
	}
	if code, out = run("path"); code != 0 || !strings.Contains(out, filepath.Join(os.Getenv("EUNOMIA_HOME"), "devices.json")) {
		t.Fatal(out)
	}
}
