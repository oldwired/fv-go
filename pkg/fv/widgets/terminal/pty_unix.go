//go:build unix

package terminal

import (
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// ptyHandle wraps creack/pty into the cross-platform shape the
// Terminal view consumes. On Unix this is a thin wrapper; on Windows
// it's backed by ConPTY (see pty_windows.go).
type ptyHandle struct {
	cmd *exec.Cmd
	tty *os.File
}

// startPTY spawns the given command attached to a fresh PTY sized
// w cols × h rows.
func startPTY(name string, args []string, env []string, cols, rows int) (*ptyHandle, error) {
	cmd := exec.Command(name, args...)
	if env != nil {
		cmd.Env = env
	}
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		return nil, err
	}
	return &ptyHandle{cmd: cmd, tty: tty}, nil
}

func (p *ptyHandle) Read(b []byte) (int, error)  { return p.tty.Read(b) }
func (p *ptyHandle) Write(b []byte) (int, error) { return p.tty.Write(b) }
func (p *ptyHandle) Close() error {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	if p.tty != nil {
		return p.tty.Close()
	}
	return nil
}

// Resize tells the kernel the PTY's window size has changed, so the
// child process gets SIGWINCH.
func (p *ptyHandle) Resize(cols, rows int) error {
	return pty.Setsize(p.tty, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

// Wait blocks until the child exits and returns its exit code.
func (p *ptyHandle) Wait() error {
	if p.cmd == nil {
		return nil
	}
	return p.cmd.Wait()
}

// _ make sure io.ReadWriteCloser is satisfied at compile time.
var _ io.ReadWriteCloser = (*ptyHandle)(nil)
