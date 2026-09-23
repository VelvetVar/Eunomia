package eunomia

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostKeyTargetsAndPaths(t *testing.T) {
	if target, _ := KeyTarget("2001:db8::1", 2222, ""); target != "[2001:db8::1]:2222" {
		t.Fatal(target)
	}
	if target, _ := KeyTarget("example", 2222, "literal-alias"); target != "literal-alias" {
		t.Fatal(target)
	}
	if _, err := KeyTarget("*", 22, ""); err == nil {
		t.Fatal("wildcard accepted")
	}
	home := filepath.Join(t.TempDir(), "Lab User")
	one := filepath.Join(home, ".ssh", "known_hosts")
	two := filepath.Join(home, ".ssh", "known_hosts2")
	target, files, err := ParseHostConfig("hostname atlas.local\nport 2222\nuserknownhostsfile "+one+" "+two, fixtureDevice(), home)
	if err != nil || target != "[atlas.local]:2222" || len(files) != 2 || files[0] != one || files[1] != two {
		t.Fatal(target, files, err)
	}
	_, files, err = ParseHostConfig("userknownhostsfile ~/.ssh/known_hosts", fixtureDevice(), home)
	if err != nil || files[0] != one {
		t.Fatal(files, err)
	}
	if _, _, err = ParseHostConfig("userknownhostsfile relative", fixtureDevice(), home); err == nil {
		t.Fatal("relative path accepted")
	}
}
func TestNativeHashedFingerprintRemoval(t *testing.T) {
	ssh, err := Executable("ssh")
	if err != nil {
		t.Skip(err)
	}
	keygen, err := Executable("ssh-keygen")
	if err != nil {
		t.Skip(err)
	}
	directory := t.TempDir()
	file := filepath.Join(directory, "known hosts")
	config := filepath.Join(directory, "config")
	target := "[192.0.2.8]:2222"
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	var wire []byte
	for _, b := range [][]byte{[]byte("ssh-ed25519"), public} {
		length := make([]byte, 4)
		binary.BigEndian.PutUint32(length, uint32(len(b)))
		wire = append(wire, length...)
		wire = append(wire, b...)
	}
	key := base64.StdEncoding.EncodeToString(wire)
	salt := make([]byte, 20)
	rand.Read(salt)
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(target))
	host := "|1|" + base64.StdEncoding.EncodeToString(salt) + "|" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	os.WriteFile(file, []byte(host+" ssh-ed25519 "+key+"\nother.local ssh-ed25519 "+key+"\n"), 0600)
	os.WriteFile(config, []byte("Host *\n  UserKnownHostsFile \""+filepath.ToSlash(file)+"\"\n"), 0600)
	d := fixtureDevice()
	d.Host = "192.0.2.8"
	d.Port = 2222
	plan, err := prepareKeys(context.Background(), d, ssh, keygen, func(ctx context.Context, file string, args []string) (ToolResult, error) {
		if args[0] == "-G" {
			args = append([]string{"-F", config}, args...)
		}
		return RunTool(ctx, file, args)
	})
	if err != nil || len(plan.Files) != 1 || plan.Target != target {
		t.Fatal(plan, err)
	}
	if err = ForgetHostKey(context.Background(), plan, nil); err != nil {
		t.Fatal(err)
	}
	contents, _ := os.ReadFile(file)
	if strings.Contains(string(contents), "|1|") || !strings.Contains(string(contents), "other.local") {
		t.Fatal(string(contents))
	}
	if _, err = os.Stat(file + ".old"); err != nil {
		t.Fatal("backup missing", err)
	}
}
