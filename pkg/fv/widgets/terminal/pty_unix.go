//go:build unix

package terminal

import (
	"io"
	"os"
	"os/exec"
	"syscall"

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
// w cols × h rows. dir, if non-empty, is the child's initial working
// directory.
func startPTY(name string, args []string, env []string, dir string, cols, rows int) (*ptyHandle, error) {
	cmd := exec.Command(name, args...)
	if env != nil {
		cmd.Env = env
	}
	if dir != "" {
		cmd.Dir = dir
	}
	// Setsid puts the child in its own session and process group.
	// Without this, killing the parent leaves grandchildren (bash →
	// vim → python) reparented to init instead of getting SIGHUP.
	// creack/pty's StartWithSize already sets Setctty=true; we ensure
	// Setsid is on too so the SIGHUP-to-pgrp path in Close() works.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
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
		// SIGHUP the entire process group first so descendants
		// (vim, less, etc. running under a shell) get a chance to
		// clean up. Negative PID = pgrp. The shell's pgid equals
		// its pid thanks to Setsid in startPTY.
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGHUP)
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
