package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Processes run one at a time in their own group. Cleanup cannot stop the user's services.
func command(ctx context.Context, env []string, dir, executable string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, executable, args...)
	c.Env, c.Dir = env, dir
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error { return syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	c.WaitDelay = 2 * time.Second
	return c
}

type boundedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.b.Len()+len(p) > 4<<20 {
		return 0, errors.New("client output exceeded limit")
	}
	return b.b.Write(p)
}
func (b *boundedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.b.Bytes())
}

func run(ctx context.Context, env []string, dir, executable string, args ...string) ([]byte, error) {
	step, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	c := command(step, env, dir, executable, args...)
	var out boundedBuffer
	c.Stdout, c.Stderr = &out, io.Discard
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("client command %s failed: %w (raw output suppressed)", args[0], err)
	}
	return out.Bytes(), nil
}
