//go:build windows

package terminal

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf16"
	"unsafe"

	"github.com/oldwired/fv-go/pkg/fv/widgets/terminal/internal/conpty"
	"golang.org/x/sys/windows"
)

// ptyHandle is a ConPTY-backed PTY: two anonymous pipes carry the
// child's stdin/stdout, a pseudo-console handle wraps both, and the
// child is started with PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE pointing
// at it. ResizePseudoConsole tells the child about WINSIZE changes.
//
// A JobObject (with KILL_ON_JOB_CLOSE) holds the child process so that
// abrupt parent termination (Ctrl-C in a debugger, crash, etc.) cleans
// up the descendant tree instead of leaving orphans behind. Graceful
// shutdown via Close() still goes through ClosePseudoConsole +
// TerminateProcess.
type ptyHandle struct {
	hPC        windows.Handle
	pipeInW    windows.Handle // parent → child stdin
	pipeOutR   windows.Handle // child → parent stdout
	inFile     *os.File       // wraps pipeInW for Write
	outFile    *os.File       // wraps pipeOutR for Read
	procHandle windows.Handle
	threadH    windows.Handle
	jobH       windows.Handle // JobObject; closes → kills child + descendants
	pid        uint32
	cmd        *exec.Cmd // synthesized for PID + Wait surface — never Started
}

func startPTY(name string, args []string, env []string, dir string, cols, rows int) (*ptyHandle, error) {
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
	// The uintptr → unsafe.Pointer conversion this needs lives in the
	// internal/conpty subpackage so CI can scope go vet's unsafeptr
	// suppression to that one package without weakening checks here.
	if err := conpty.SetPseudoConsoleAttribute(attrList, hPC); err != nil {
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
	// envBlockBuf holds the backing storage for envBlock. Must remain
	// in scope until CreateProcess returns — the Win32 API reads from
	// the pointer during process creation, and Go's GC must not move
	// or collect this slice before then.
	var envBlockBuf []uint16
	if len(env) > 0 {
		envBlockBuf, err = utf16EnvBlock(env)
		if err != nil {
			windows.ClosePseudoConsole(hPC)
			windows.CloseHandle(stdinParentW)
			windows.CloseHandle(stdoutParentR)
			return nil, err
		}
		envBlock = &envBlockBuf[0]
	}

	var dirP *uint16
	if dir != "" {
		dirP, err = windows.UTF16PtrFromString(dir)
		if err != nil {
			windows.ClosePseudoConsole(hPC)
			windows.CloseHandle(stdinParentW)
			windows.CloseHandle(stdoutParentR)
			return nil, err
		}
	}

	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	cpErr := windows.CreateProcess(
		nil, cmdLineP, nil, nil, false,
		flags, envBlock, dirP, &si.StartupInfo, &pi,
	)
	// Keep envBlockBuf alive across the CreateProcess call. Without
	// this, an aggressive optimizer could deallocate the slice while
	// the kernel is still reading the environment block.
	runtime.KeepAlive(envBlockBuf)
	if cpErr != nil {
		windows.ClosePseudoConsole(hPC)
		windows.CloseHandle(stdinParentW)
		windows.CloseHandle(stdoutParentR)
		return nil, fmt.Errorf("CreateProcess: %w", cpErr)
	}

	// Stuff the new process into a JobObject set to terminate every
	// member on handle close. The OS auto-closes the handle when the
	// parent dies — including ungraceful exits — so descendants (the
	// shell, programs it spawns) don't escape into init.
	jobH, jobErr := windows.CreateJobObject(nil, nil)
	if jobErr == nil {
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		if _, e := windows.SetInformationJobObject(
			jobH,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		); e == nil {
			_ = windows.AssignProcessToJobObject(jobH, pi.Process)
		}
	}

	p := &ptyHandle{
		hPC:        hPC,
		pipeInW:    stdinParentW,
		pipeOutR:   stdoutParentR,
		inFile:     os.NewFile(uintptr(stdinParentW), "conpty-in"),
		outFile:    os.NewFile(uintptr(stdoutParentR), "conpty-out"),
		procHandle: pi.Process,
		threadH:    pi.Thread,
		jobH:       jobH,
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
	if p.jobH != 0 {
		// Closing the job kills any remaining members. Last line of
		// defense against descendant processes outliving the pane.
		windows.CloseHandle(p.jobH)
		p.jobH = 0
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

// quoteWindowsArg quotes a single argument into the form
// CommandLineToArgvW will round-trip. The Win32 rules (per MSDN
// "Parsing C++ Command-Line Arguments"):
//
//   - 2n backslashes followed by a quotation mark produce n
//     backslashes followed by a begin/end quote.
//   - 2n+1 backslashes followed by a quotation mark produce n
//     backslashes followed by a literal quotation mark.
//   - n backslashes not followed by a quotation mark produce n
//     literal backslashes.
//
// Naive `strings.ReplaceAll(a, `"`, `\"`)` quoting was the previous
// implementation; it failed on paths like `C:\Program Files\Foo\`
// because the trailing backslash before the closing quote would be
// re-interpreted as escaping the quote, dropping the closing wrapper.
func quoteWindowsArg(a string) string {
	if a == "" {
		return `""`
	}
	// Fast path: nothing special.
	if !strings.ContainsAny(a, ` \t"`+"\n\v") {
		return a
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for i := 0; i < len(a); i++ {
		c := a[i]
		switch c {
		case '\\':
			backslashes++
		case '"':
			// Double every preceding backslash plus this one, then
			// emit the escaped quote.
			for j := 0; j < 2*backslashes+1; j++ {
				b.WriteByte('\\')
			}
			b.WriteByte('"')
			backslashes = 0
		default:
			for j := 0; j < backslashes; j++ {
				b.WriteByte('\\')
			}
			backslashes = 0
			b.WriteByte(c)
		}
	}
	// Trailing backslashes need to be doubled so the closing quote
	// isn't accidentally escaped.
	for j := 0; j < 2*backslashes; j++ {
		b.WriteByte('\\')
	}
	b.WriteByte('"')
	return b.String()
}

// windowsCommandLine quotes name + args into the single-string form
// CreateProcessW expects, using the full CommandLineToArgvW inverse so
// trailing-backslash paths and embedded quotes round-trip correctly.
func windowsCommandLine(name string, args []string) string {
	var b strings.Builder
	b.WriteString(quoteWindowsArg(name))
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(quoteWindowsArg(a))
	}
	return b.String()
}

// utf16EnvBlock turns a slice of "K=V" strings into a UTF-16 environment
// block (each entry NUL-terminated, the block as a whole double-NUL
// terminated) suitable for CreateProcessW's lpEnvironment parameter
// (with CREATE_UNICODE_ENVIRONMENT). Returns the backing slice — the
// caller must keep it alive (via runtime.KeepAlive) until CreateProcess
// returns. NUL bytes inside an entry are rejected; without this guard
// the syscall would silently truncate the block.
//
// The earlier implementation built one large NUL-joined string and
// called syscall.UTF16FromString on it, which stops at the first NUL
// — so only the first env var was actually passed to the child. That
// was the headline ConPTY bug in the project review.
func utf16EnvBlock(env []string) ([]uint16, error) {
	var block []uint16
	for _, e := range env {
		if strings.IndexByte(e, 0) >= 0 {
			return nil, errors.New("env entry contains NUL byte")
		}
		block = append(block, utf16.Encode([]rune(e))...)
		block = append(block, 0)
	}
	// Final terminator: empty entry signals end-of-block.
	block = append(block, 0)
	return block, nil
}
