package eunomia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type HostKeyPlan struct {
	Device Device
	Target string
	Files  []string
	Keygen string
}
type ToolResult struct {
	Code           int
	Stdout, Stderr string
}
type ToolRunner func(context.Context, string, []string) (ToolResult, error)

func RunTool(ctx context.Context, file string, args []string) (ToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, file, args...)
	cmd.WaitDelay = time.Second
	quietCommand(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := ToolResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			result.Code = exit.ExitCode()
			return result, nil
		}
		return result, err
	}
	return result, nil
}

var unsafeTarget = regexp.MustCompile(`[\s*?!,]`)

func KeyTarget(host string, port int, alias string) (string, error) {
	target := alias
	if target == "" {
		target = strings.ToLower(host)
		if port != 22 {
			target = "[" + target + "]:" + strconv.Itoa(port)
		}
	}
	if target == "" || safe(target) != target || unsafeTarget.MatchString(target) {
		return "", errors.New("configured host-key target is not a single host; no keys were changed")
	}
	return target, nil
}

// OpenSSH -G prints unquoted paths; recognize absolute path boundaries while preserving spaces in home directories.
var pathBoundary = regexp.MustCompile(`\s+(?:[a-zA-Z]:[\\/]|/|~[\\/]|\$\{|%d)`)
var envPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func ParseHostConfig(output string, d Device, home string) (string, []string, error) {
	config := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if ok {
			config[key] = strings.TrimSpace(value)
		}
	}
	host := config["hostname"]
	if host == "" {
		host = d.Host
	}
	port := d.Port
	if p, err := strconv.Atoi(config["port"]); err == nil {
		port = p
	}
	alias := config["hostkeyalias"]
	target, err := KeyTarget(host, port, alias)
	if err != nil {
		return "", nil, err
	}
	raw := config["userknownhostsfile"]
	if raw == "none" {
		return target, nil, nil
	}
	if raw == "" {
		return "", nil, errors.New("SSH did not report its UserKnownHostsFile")
	}
	boundaries := pathBoundary.FindAllStringIndex(raw, -1)
	parts := []string{}
	start := 0
	for _, boundary := range boundaries {
		parts = append(parts, raw[start:boundary[0]])
		start = boundary[0]
		for start < len(raw) && (raw[start] == ' ' || raw[start] == '\t') {
			start++
		}
	}
	parts = append(parts, raw[start:])
	files := []string{}
	seen := map[string]bool{}
	for _, part := range parts {
		file := strings.Trim(strings.TrimSpace(part), "\"")
		var expansionErr error
		file = envPattern.ReplaceAllStringFunc(file, func(key string) string {
			value, ok := os.LookupEnv(key[2 : len(key)-1])
			if !ok {
				expansionErr = fmt.Errorf("undefined SSH path variable %s", key)
			}
			return value
		})
		if expansionErr != nil {
			return "", nil, expansionErr
		}
		var expanded strings.Builder
		for i := 0; i < len(file); i++ {
			if file[i] != '%' {
				expanded.WriteByte(file[i])
				continue
			}
			i++
			if i >= len(file) {
				return "", nil, errors.New("unsupported SSH path token")
			}
			value := ""
			switch file[i] {
			case '%':
				value = "%"
			case 'd':
				value = home
			case 'h':
				value = host
			case 'n':
				value = d.Host
			case 'k':
				value = alias
				if value == "" {
					value = d.Host
				}
			case 'p':
				value = strconv.Itoa(port)
			case 'r':
				value = d.Username
			default:
				return "", nil, errors.New("unsupported SSH path token; no keys were changed")
			}
			expanded.WriteString(value)
		}
		file = expanded.String()
		if strings.HasPrefix(file, "~/") || strings.HasPrefix(file, `~\`) {
			file = filepath.Join(home, file[2:])
		}
		if !filepath.IsAbs(file) {
			return "", nil, errors.New("UserKnownHostsFile must use absolute paths; no keys were changed")
		}
		file = filepath.Clean(file)
		if !seen[file] {
			seen[file] = true
			files = append(files, file)
		}
	}
	return target, files, nil
}
func PrepareHostKeyReset(ctx context.Context, d Device) (HostKeyPlan, error) {
	ssh, err := Executable("ssh")
	if err != nil {
		return HostKeyPlan{}, err
	}
	keygen, err := Executable("ssh-keygen")
	if err != nil {
		return HostKeyPlan{}, err
	}
	return prepareKeys(ctx, d, ssh, keygen, RunTool)
}
func prepareKeys(ctx context.Context, d Device, ssh, keygen string, run ToolRunner) (HostKeyPlan, error) {
	plan := HostKeyPlan{Device: d, Keygen: keygen}
	config, err := run(ctx, ssh, append([]string{"-G"}, SSHArgs(d)...))
	if err != nil {
		return plan, err
	}
	if config.Code != 0 {
		return plan, fmt.Errorf("cannot read SSH configuration: %s", safe(config.Stderr))
	}
	home, _ := os.UserHomeDir()
	target, files, err := ParseHostConfig(config.Stdout, d, home)
	if err != nil {
		return plan, err
	}
	plan.Target = target
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return plan, fmt.Errorf("cannot inspect known_hosts: %w", err)
		}
		found, err := run(ctx, keygen, []string{"-F", target, "-f", file})
		if err != nil {
			return plan, err
		}
		if found.Code == 0 && strings.TrimSpace(found.Stdout) != "" {
			plan.Files = append(plan.Files, file)
		} else if found.Code != 1 || strings.TrimSpace(found.Stderr) != "" {
			return plan, fmt.Errorf("cannot inspect known_hosts (exit %d): %s", found.Code, safe(found.Stderr))
		}
	}
	return plan, nil
}
func ForgetHostKey(ctx context.Context, plan HostKeyPlan, run ToolRunner) error {
	if run == nil {
		run = RunTool
	}
	for i, file := range plan.Files {
		result, err := run(ctx, plan.Keygen, []string{"-R", plan.Target, "-f", file})
		if err != nil {
			return fmt.Errorf("updated %d files; %w", i, err)
		}
		if result.Code != 0 {
			return fmt.Errorf("updated %d files; cannot update %s: %s", i, file, safe(result.Stderr))
		}
	}
	return nil
}
