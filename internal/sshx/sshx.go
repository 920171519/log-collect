package sshx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Client wraps an SSH transport and owns its keepalive goroutine. The embedded
// ssh.Client is safe for concurrent sessions and direct-tcpip channels.
type Client struct {
	*ssh.Client
	addr      string
	done      chan struct{}
	closeOnce sync.Once
}

// Dial opens a direct SSH connection. The same config is used for the cloud
// host and for devices reached through SSH direct-tcpip jumps.
func Dial(ctx context.Context, addr, username, password string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return handshake(ctx, conn, addr, username, password, timeout)
}

// DialVia opens a TCP channel through jump, then performs SSH over that
// channel. This is the Go equivalent of ProxyJump and is used for
// cloud -> device A -> device B connectivity.
func DialVia(ctx context.Context, jump *Client, addr, username, password string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if jump == nil {
		return nil, errors.New("跳板连接不存在")
	}
	type dialResult struct {
		conn net.Conn
		err  error
	}
	resultCh := make(chan dialResult, 1)
	go func() {
		conn, err := jump.Client.Dial("tcp", addr)
		resultCh <- dialResult{conn: conn, err: err}
	}()
	var conn net.Conn
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultCh:
		if res.err != nil {
			return nil, res.err
		}
		conn = res.conn
	}
	return handshake(ctx, conn, addr, username, password, timeout)
}

func handshake(ctx context.Context, conn net.Conn, addr, username, password string, timeout time.Duration) (*Client, error) {
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	cfg := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = password
				}
				return answers, nil
			}),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
		// Some internal devices only offer the legacy ssh-rsa host key
		// signature. rsa-sha2 is still preferred by the handshake when offered.
		HostKeyAlgorithms: []string{
			ssh.KeyAlgoRSASHA256,
			ssh.KeyAlgoRSASHA512,
			ssh.KeyAlgoRSA,
			ssh.KeyAlgoECDSA256,
			ssh.KeyAlgoECDSA384,
			ssh.KeyAlgoECDSA521,
			ssh.KeyAlgoED25519,
		},
	}
	handshakeDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// Interrupt a stalled TCP / SSH handshake promptly when the user
			// stops the collection or the run deadline expires.
			_ = conn.Close()
		case <-handshakeDone:
		}
	}()
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	close(handshakeDone)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	c := &Client{
		Client: ssh.NewClient(clientConn, chans, reqs),
		addr:   addr,
		done:   make(chan struct{}),
	}
	go c.keepAlive()
	return c, nil
}

func (c *Client) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			_, _, err := c.Client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				return
			}
		}
	}
}

func (c *Client) Addr() string { return c.addr }

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	var err error
	c.closeOnce.Do(func() {
		close(c.done)
		err = c.Client.Close()
	})
	return err
}

// DialContext wraps ssh.Client.Dial in context cancellation. The SSH client
// itself does not offer a context-aware variant.
func (c *Client) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if c == nil {
		return nil, errors.New("SSH 连接不存在")
	}
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := c.Client.Dial(network, addr)
		ch <- result{conn: conn, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.conn, res.err
	}
}

// CheckAddress is a small helper used by callers that need a consistent error
// prefix without exposing credentials.
func CheckAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("SSH 地址 %q 无效：%w", addr, err)
	}
	if host == "" || port == "" {
		return fmt.Errorf("SSH 地址 %q 无效", addr)
	}
	return nil
}
