package collector

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type remoteResult struct {
	ExitStatus int
	MarkerSeen bool
}

type outputLine struct {
	stream string
	text   string
}

// runRemoteCommand streams a remote command's output and watches for the
// completion marker. Returning normally means the SSH channel closed and all
// available output was delivered. A non-zero ExitStatus is not treated as a
// transport error because the marker/status line is more reliable for the
// collection protocol.
func runRemoteCommand(
	ctx context.Context,
	client *ssh.Client,
	command string,
	marker string,
	sink func(stream, line string),
) (remoteResult, error) {
	result := remoteResult{ExitStatus: -1}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if client == nil {
		return result, errors.New("SSH 连接不存在")
	}
	session, err := client.NewSession()
	if err != nil {
		return result, fmt.Errorf("创建 SSH 会话失败：%w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return result, fmt.Errorf("获取标准输出失败：%w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return result, fmt.Errorf("获取错误输出失败：%w", err)
	}
	if err := session.Start(command); err != nil {
		return result, fmt.Errorf("启动远程命令失败：%w", err)
	}

	lineCh := make(chan outputLine, 256)
	stopPumps := make(chan struct{})
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(stopPumps) }) }
	defer stop()

	var wg sync.WaitGroup
	wg.Add(2)
	pump := func(r io.Reader, stream string) {
		defer wg.Done()
		reader := bufio.NewReaderSize(r, 64*1024)
		for {
			line, readErr := reader.ReadString('\n')
			if len(line) > 0 {
				text := strings.TrimRight(line, "\r\n")
				select {
				case lineCh <- outputLine{stream: stream, text: text}:
				case <-stopPumps:
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}
	go pump(stdout, "stdout")
	go pump(stderr, "stderr")
	go func() {
		wg.Wait()
		close(lineCh)
	}()

	waitErrCh := make(chan error, 1)
	go func() {
		waitErrCh <- session.Wait()
	}()

	var (
		waitErr         error
		exited          bool
		markerSeen      bool
		cancelRequested bool
		ctxDone         = ctx.Done()
		forceTimer      <-chan time.Time
		killTimer       *time.Timer
	)

loop:
	for {
		if exited && lineCh == nil {
			break
		}
		select {
		case line, ok := <-lineCh:
			if !ok {
				lineCh = nil
				continue
			}
			if marker != "" && isMarkerLine(line.text, marker) {
				markerSeen = true
				continue
			}
			if sink != nil {
				sink(line.stream, line.text)
			}
		case err := <-waitErrCh:
			waitErr = err
			exited = true
			waitErrCh = nil
		case <-ctxDone:
			ctxDone = nil
			cancelRequested = true
			// Ask the remote sshd to terminate the process group, then close
			// the channel if it has not exited shortly afterwards.
			_ = session.Signal(ssh.SIGTERM)
			killTimer = time.AfterFunc(1500*time.Millisecond, func() {
				_ = session.Signal(ssh.SIGKILL)
				_ = session.Close()
			})
			forceTimer = time.After(4 * time.Second)
		case <-forceTimer:
			forceTimer = nil
			_ = session.Close()
			// Do not let a broken transport keep stop/start buttons pending
			// forever. Pumps are stopped below and all late lines are dropped.
			break loop
		}
	}
	if killTimer != nil {
		killTimer.Stop()
	}
	stop()
	_ = session.Close()

	result.MarkerSeen = markerSeen
	if cancelRequested {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		return result, context.Canceled
	}
	if waitErr == nil {
		result.ExitStatus = 0
	} else {
		var exitErr *ssh.ExitError
		if errors.As(waitErr, &exitErr) {
			result.ExitStatus = exitErr.ExitStatus()
		} else {
			return result, fmt.Errorf("远程命令执行失败：%w", waitErr)
		}
	}
	return result, nil
}

func isMarkerLine(line, marker string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == marker || strings.HasPrefix(trimmed, marker+":")
}
