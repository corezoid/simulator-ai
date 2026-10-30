package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

type peer struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	scanner *bufio.Scanner
	next    int
}
type reply struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

func startPeer(ctx context.Context, env []string, dir, executable string, args ...string) (*peer, error) {
	c := command(ctx, env, dir, executable, args...)
	c.Stderr = io.Discard
	in, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = c.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	return &peer{cmd: c, in: in, scanner: scanner}, nil
}
func (p *peer) close() { _ = p.in.Close(); _ = p.cmd.Cancel(); _ = p.cmd.Wait() }
func (p *peer) call(method string, params any, result any) error {
	p.next++
	if err := json.NewEncoder(p.in).Encode(struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{p.next, method, params}); err != nil {
		return err
	}
	for p.scanner.Scan() {
		var r reply
		if err := json.Unmarshal(p.scanner.Bytes(), &r); err != nil {
			return errors.New("client emitted invalid protocol")
		}
		if r.Method != "" { // No approval, elicitation or other server requests are authorized by this smoke.
			if r.ID != 0 {
				return errors.New("unexpected client request; smoke does not approve actions")
			}
			continue
		}
		if r.ID != p.next {
			return errors.New("client reply identity mismatch")
		}
		if len(r.Error) > 0 && string(r.Error) != "null" {
			return fmt.Errorf("client protocol %s failed (details suppressed)", method)
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(r.Result, result)
	}
	if err := p.scanner.Err(); err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}
