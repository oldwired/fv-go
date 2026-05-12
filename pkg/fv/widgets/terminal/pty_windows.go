//go:build windows

package terminal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ptyHandle is a ConPTY-backed PTY: two anonymous pipes carry the
// child's stdin/stdout, a pseudo-console handle wraps both, and the
// child is started with PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE pointing
// at it. ResizePseudoConsole tells the child about WINSIZE changes.
type ptyHandle struct {
	hPC        windows.Handle
	pipeInW    windows.Handle // parent → child stdin
	pipeOutR   windows.Handle // child → parent stdout
	inFile     *os.File       // wraps pipeInW for Write
	outFile    *os.File       // wraps pipeOutR for Read
	procHandle windows.Handle
	threadH    windows.Handle
	pid        uint32
	cmd        *exec.Cmd // synthesized for PID + Wait surface — never Started
}

func startPTY(name string, args []string, env []string, cols, rows int) (*ptyHandle, error) {
	// Create two pipes. Each call gives us (read, write); we'll keep
	// the parent-side end and pass the child-side end to the
	// pseudo-console. Pseudo-console takes ownership of the handles
	// it's given (it will CloseHandle them when ClosePseudoConsole
	// fires), so we MUST hand it the child ends and close our local
	// copies after CreatePseudoConsole succeeds.
	var stdinChildR, stdinParentW windows.Handle
	if err := windows.CreatePipe(&stdinChildR, &stdinParentW, nil, 0); err != nil {
		return nil, fmt.Errorf("pipe (stdin): %w", err)
	}
	var stdoutParentR, stdoutChildW windows.Handle
	if err := windows.CreatePipe(&stdoutParentR, &stdoutChildW, nil, 0); err != nil {
		windows.CloseHandle(stdinChildR)
		windows.CloseHandle(stdinParentW)
		return nil, fmt.Errorf("pipe (stdout): %w", err)
	}

	var hPC windows.Handle
	size := windows.Coord{X: int16(cols), Y: int16(rows)}
	if err := windows.CreatePseudoConsole(size, stdinChildR, stdoutChildW, 0, &hPC); err != nil {
		windows.CloseHandle(stdinChildR)
		windows.CloseHandle(stdinParentW)
		windows.CloseHandle(stdoutParentR)
		windows.CloseHandle(stdoutChildW)
		return nil, fmt.Errorf("CreatePseudoConsole: %w", err)
	}
	// Our copies of the child-side ends are no longer needed — the
	// pseudo-console owns them now.
	windows.CloseHandle(stdinChildR)
	windows.CloseHandle(stdoutChildW)

	// Build the proc-thread attribute list with the pseudo-console
	// pointer. Required so CreateProcessW with EXTENDED_STARTUPINFO_
	// PRESENT routes stdin/stdout through ConPTY rather than the
	// inheriting parent console.
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(stdinParentW)
		windows.CloseHandle(stdoutParentR)
		return nil, fmt.Errorf("NewProcThreadAttributeList: %w", err)
	}
	if err := attrList.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		unsafe.Pointer(hPC),
		unsafe.Sizeof(hPC),
	); err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(stdinParentW)
		windows.CloseHandle(stdoutParentR)
		return nil, fmt.Errorf("UpdateProcThreadAttribute: %w", err)
	}
	defer attrList.Delete()

	si := &windows.StartupInfoEx{}
	si.StartupInfo.Cb = uint32(unsafe.Sizeof(*si))
	si.ProcThreadAttributeList = attrList.List()

	// Build the command line. Windows uses a single command-string
	// rather than argv; quote each arg.
	cmdLine := windowsCommandLine(name, args)
	cmdLineP, err := windows.UTF16PtrFromString(cmdLine)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(stdinParentW)
		windows.CloseHandle(stdoutParentR)
		return nil, err
	}
	var envBlock *uint16
	if len(env) > 0 {
		envBlock, err = utf16EnvBlock(env)
		if err != nil {
			windows.ClosePseudoConsole(hPC)
			windows.CloseHandle(stdinParentW)
			windows.CloseHandle(stdoutParentR)
			return nil, err
		}
	}

	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	if err := windows.CreateProcess(
		nil, cmdLineP, nil, nil, false,
		flags, envBlock, nil, &si.StartupInfo, &pi,
	); err != nil {
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(stdinParentW)
		windows.CloseHandle(stdoutParentR)
		return nil, fmt.Errorf("CreateProcess: %w", err)
	}

	p := &ptyHandle{
		hPC:        hPC,
		pipeInW:    stdinParentW,
		pipeOutR:   stdoutParentR,
		inFile:     os.NewFile(uintptr(stdinParentW), "conpty-in"),
		outFile:    os.NewFile(uintptr(stdoutParentR), "conpty-out"),
		procHandle: pi.Process,
		threadH:    pi.Thread,
		pid:        pi.ProcessId,
	}
	// Synthesize an *exec.Cmd shell so the rest of the package can
	// pull PID off cmd.Process.Pid the same way it does on Unix. We
	// don't actually Start this cmd — it just carries the values.
	p.cmd = &exec.Cmd{
		Path:    name,
		Args:    append([]string{name}, args...),
		Process: &os.Process{Pid: int(pi.ProcessId)},
	}
	return p, nil
}

func (p *ptyHandle) Read(b []byte) (int, error)  { return p.outFile.Read(b) }
func (p *ptyHandle) Write(b []byte) (int, error) { return p.inFile.Write(b) }

func (p *ptyHandle) Close() error {
	// Order matters: closing the pseudo-console signals EOF to the
	// child and unblocks any pending read on the parent's stdout end.
	if p.hPC != 0 {
		windows.ClosePseudoConsole(p.hPC)
		p.hPC = 0
	}
	if p.inFile != nil {
		_ = p.inFile.Close()
	}
	if p.outFile != nil {
		_ = p.outFile.Close()
	}
	if p.procHandle != 0 {
		_ = windows.TerminateProcess(p.procHandle, 1)
		windows.CloseHandle(p.procHandle)
		p.procHandle = 0
	}
	if p.threadH != 0 {
		windows.CloseHandle(p.threadH)
		p.threadH = 0
	}
	return nil
}

func (p *ptyHandle) Resize(cols, rows int) error {
	if p.hPC == 0 {
		return errors.New("pty: closed")
	}
	return windows.ResizePseudoConsole(p.hPC, windows.Coord{X: int16(cols), Y: int16(rows)})
}

func (p *ptyHandle) Wait() error {
	if p.procHandle == 0 {
		return errors.New("pty: no process")
	}
	if _, err := windows.WaitForSingleObject(p.procHandle, windows.INFINITE); err != nil {
		return err
	}
	var code uint32
	_ = windows.GetExitCodeProcess(p.procHandle, &code)
	if code != 0 {
		return fmt.Errorf("exit status %d", code)
	}
	return nil
}

// windowsCommandLine quotes name + args into the single-string form
// CreateProcessW expects. Each token gets wrapped in quotes and inner
// quotes escaped with backslash — the standard CommandLineToArgvW
// dance. Good enough for the launchers we'd actually run (shells,
// editors); skip the corner cases (embedded null, trailing backslash
// before a quote) the package doesn't deal in.
func windowsCommandLine(name string, args []string) string {
	var b strings.Builder
	b.WriteString(`"`)
	b.WriteString(strings.ReplaceAll(name, `"`, `\"`))
	b.WriteString(`"`)
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(`"`)
		b.WriteString(strings.ReplaceAll(a, `"`, `\"`))
		b.WriteString(`"`)
	}
	return b.String()
}

// utf16EnvBlock turns a slice of "K=V" strings into a UTF-16 block
// with a double-null terminator, suitable for CreateProcessW's
// lpEnvironment parameter (with CREATE_UNICODE_ENVIRONMENT).
func utf16EnvBlock(env []string) (*uint16, error) {
	var sb strings.Builder
	for _, e := range env {
		sb.WriteString(e)
		sb.WriteByte(0)
	}
	sb.WriteByte(0)
	utf16, err := syscall.UTF16FromString(sb.String())
	if err != nil {
		return nil, err
	}
	return &utf16[0], nil
}
