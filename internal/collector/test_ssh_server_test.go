package collector

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// testSSHServer is a minimal SSH/SFTP server used to exercise the complete
// cloud -> device A -> device B jump path without requiring real hardware.
type testSSHServer struct {
	name     string
	password string
	listener net.Listener
	config   *ssh.ServerConfig

	mu       sync.Mutex
	conns    map[net.Conn]struct{}
	closed   bool
	forwards []string
	accepts  int

	wg sync.WaitGroup
}

func newED25519Signer(t *testing.T) ssh.Signer {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	return signer
}

func startTestSSHServer(t *testing.T, name, password string, signer ssh.Signer) *testSSHServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &testSSHServer{
		name:     name,
		password: password,
		listener: listener,
		conns:    make(map[net.Conn]struct{}),
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(metadata ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) == s.password {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("invalid password for %s", metadata.User())
		},
	}
	config.AddHostKey(signer)
	s.config = config

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				_ = conn.Close()
				return
			}
			s.conns[conn] = struct{}{}
			s.accepts++
			s.mu.Unlock()
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.handleConn(conn)
				s.mu.Lock()
				delete(s.conns, conn)
				s.mu.Unlock()
			}()
		}
	}()

	t.Cleanup(func() {
		s.mu.Lock()
		if !s.closed {
			s.closed = true
			_ = s.listener.Close()
			for conn := range s.conns {
				_ = conn.Close()
			}
		}
		s.mu.Unlock()
	})
	return s
}

func (s *testSSHServer) addr() string {
	return s.listener.Addr().String()
}

func (s *testSSHServer) port() int {
	_, portText, _ := net.SplitHostPort(s.addr())
	port, _ := strconv.Atoi(portText)
	return port
}

func (s *testSSHServer) acceptCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accepts
}

func (s *testSSHServer) forwardedTargets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.forwards...)
}

func (s *testSSHServer) handleConn(conn net.Conn) {
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)
	for newChannel := range chans {
		switch newChannel.ChannelType() {
		case "session":
			go s.handleSession(newChannel)
		case "direct-tcpip":
			go s.handleDirectTCPIP(newChannel)
		default:
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel")
		}
	}
}

func (s *testSSHServer) handleSession(newChannel ssh.NewChannel) {
	channel, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	var (
		procMu sync.Mutex
		proc   *os.Process
	)
	reply := func(req *ssh.Request, ok bool) {
		if req.WantReply {
			_ = req.Reply(ok, nil)
		}
	}

	for req := range requests {
		switch req.Type {
		case "exec":
			var payload struct {
				Command string
			}
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
				reply(req, false)
				_ = channel.Close()
				return
			}
			reply(req, true)
			go func(command string) {
				cmd := exec.Command("/bin/sh", "-c", command)
				cmd.Env = append(os.Environ(), "LC_ALL=C")
				cmd.Stdout = channel
				cmd.Stderr = channel.Stderr()
				if err := cmd.Start(); err != nil {
					fmt.Fprintln(channel.Stderr(), err)
					sendExitStatus(channel, 1)
					_ = channel.Close()
					return
				}
				procMu.Lock()
				proc = cmd.Process
				procMu.Unlock()
				err := cmd.Wait()
				procMu.Lock()
				proc = nil
				procMu.Unlock()
				status := 0
				if err != nil {
					if exitError, ok := err.(*exec.ExitError); ok {
						status = exitError.ExitCode()
					} else {
						status = 1
						fmt.Fprintln(channel.Stderr(), err)
					}
				}
				sendExitStatus(channel, status)
				_ = channel.Close()
			}(payload.Command)
		case "subsystem":
			var payload struct {
				Name string
			}
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil || payload.Name != "sftp" {
				reply(req, false)
				_ = channel.Close()
				return
			}
			reply(req, true)
			server, err := sftp.NewServer(channel)
			if err != nil {
				sendExitStatus(channel, 1)
				_ = channel.Close()
				return
			}
			err = server.Serve()
			_ = server.Close()
			if err != nil && err != io.EOF {
				sendExitStatus(channel, 1)
			} else {
				sendExitStatus(channel, 0)
			}
			_ = channel.Close()
			return
		case "signal":
			var payload struct {
				Signal string
			}
			_ = ssh.Unmarshal(req.Payload, &payload)
			procMu.Lock()
			p := proc
			procMu.Unlock()
			if p != nil {
				signalName := strings.ToUpper(strings.TrimPrefix(payload.Signal, "SIG"))
				switch signalName {
				case "TERM":
					_ = p.Signal(syscall.SIGTERM)
				case "KILL":
					_ = p.Signal(syscall.SIGKILL)
				case "INT":
					_ = p.Signal(syscall.SIGINT)
				}
			}
			reply(req, true)
		case "env", "pty-req", "window-change":
			reply(req, true)
		default:
			reply(req, false)
		}
	}
	_ = channel.Close()
}

func sendExitStatus(channel ssh.Channel, status int) {
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct {
		Status uint32
	}{Status: uint32(status)}))
}

func (s *testSSHServer) handleDirectTCPIP(newChannel ssh.NewChannel) {
	var payload struct {
		HostToConnect     string
		PortToConnect     uint32
		OriginatorAddress string
		OriginatorPort    uint32
	}
	if err := ssh.Unmarshal(newChannel.ExtraData(), &payload); err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, "bad direct-tcpip payload")
		return
	}
	targetAddr := net.JoinHostPort(payload.HostToConnect, strconv.Itoa(int(payload.PortToConnect)))
	conn, err := net.DialTimeout("tcp", targetAddr, 3e9)
	if err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	channel, requests, err := newChannel.Accept()
	if err != nil {
		_ = conn.Close()
		return
	}
	go ssh.DiscardRequests(requests)
	s.mu.Lock()
	s.forwards = append(s.forwards, targetAddr)
	s.mu.Unlock()

	go func() {
		_, _ = io.Copy(channel, conn)
		_ = channel.Close()
	}()
	_, _ = io.Copy(conn, channel)
	_ = conn.Close()
}
