package eunomia

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type runState struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}
type controlRequest struct {
	Action string `json:"action"`
	Token  string `json:"token"`
}
type controlReply struct {
	OK bool `json:"ok"`
}
type Control struct {
	directory string
	state     runState
	listener  net.Listener
	once      sync.Once
	stop      func()
}

func readRunState(directory string) (runState, error) {
	var state runState
	file, err := os.Open(filepath.Join(directory, "running.json"))
	if err != nil {
		return state, err
	}
	defer file.Close()
	bytes, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return state, err
	}
	if len(bytes) > 4096 || json.Unmarshal(bytes, &state) != nil || state.PID < 1 || state.Port < 1 || state.Port > 65535 || len(state.Token) != 64 {
		return state, errors.New("invalid running.json; close Eunomia before removing this runtime record")
	}
	if _, err = hex.DecodeString(state.Token); err != nil {
		return state, errors.New("invalid control token")
	}
	return state, nil
}
func removeRunState(directory, token string) {
	if state, err := readRunState(directory); err == nil && state.Token == token {
		os.Remove(filepath.Join(directory, "running.json"))
	}
}
func StartControl(directory string, stop func()) (*Control, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		listener.Close()
		return nil, err
	}
	state := runState{os.Getpid(), listener.Addr().(*net.TCPAddr).Port, hex.EncodeToString(secret[:])}
	bytes, _ := json.Marshal(state)
	file := filepath.Join(directory, "running.json")
	written := false
	for attempt := 0; attempt < 2; attempt++ {
		handle, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, err = handle.Write(bytes)
			handle.Close()
			if err != nil {
				os.Remove(file)
				listener.Close()
				return nil, err
			}
			written = true
			break
		}
		if !errors.Is(err, os.ErrExist) {
			listener.Close()
			return nil, err
		}
		time.Sleep(100 * time.Millisecond)
		previous, err := readRunState(directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			listener.Close()
			return nil, err
		}
		if processAlive(previous.PID) {
			listener.Close()
			return nil, errors.New("Eunomia is already running; use its terminal or run eunomia down first")
		}
		removeRunState(directory, previous.Token)
	}
	if !written {
		listener.Close()
		return nil, errors.New("another Eunomia instance is starting; retry in a moment")
	}
	c := &Control{directory: directory, state: state, listener: listener, stop: stop}
	go c.serve()
	return c, nil
}
func (c *Control) serve() {
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(3 * time.Second))
			line, err := bufio.NewReader(io.LimitReader(conn, 4097)).ReadBytes('\n')
			if err != nil || len(line) > 4096 {
				return
			}
			var request controlRequest
			if json.Unmarshal(line, &request) != nil {
				return
			}
			ok := subtle.ConstantTimeCompare([]byte(request.Token), []byte(c.state.Token)) == 1 && (request.Action == "down" || request.Action == "status")
			json.NewEncoder(conn).Encode(controlReply{ok})
			if ok && request.Action == "down" {
				c.stop()
			}
		}()
	}
}
func (c *Control) Close() {
	c.once.Do(func() { c.listener.Close(); removeRunState(c.directory, c.state.Token) })
}
func StopRunning(directory string) (bool, error) {
	state, err := readRunState(directory)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	conn, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", state.Port), 3*time.Second)
	if err == nil {
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		err = json.NewEncoder(conn).Encode(controlRequest{"down", state.Token})
		if err == nil {
			var response controlReply
			err = json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&response)
			if err == nil && !response.OK {
				err = errors.New("control authentication failed")
			}
		}
	}
	if err != nil {
		if !processAlive(state.PID) {
			removeRunState(directory, state.Token)
			return false, nil
		}
		return false, fmt.Errorf("could not stop Eunomia safely: %w; close it in its terminal", err)
	}
	return true, nil
}
