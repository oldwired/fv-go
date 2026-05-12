//go:build windows

package terminal

import "errors"

// ptyHandle on Windows would use ConPTY (CreatePseudoConsole +
// ResizePseudoConsole). Wiring that up requires a non-trivial chunk
// of golang.org/x/sys/windows plumbing — deferred to a follow-up.
//
// Until then, Terminal.Run returns this error and the demo's menu
// entry shows it via msgbox so users get a clear "not yet on Windows"
// message instead of silent breakage.
type ptyHandle struct{}

func startPTY(name string, args []string, env []string, cols, rows int) (*ptyHandle, error) {
	return nil, errors.New("terminal: ConPTY support not implemented on Windows yet")
}

func (p *ptyHandle) Read(b []byte) (int, error)  { return 0, errors.New("not implemented") }
func (p *ptyHandle) Write(b []byte) (int, error) { return 0, errors.New("not implemented") }
func (p *ptyHandle) Close() error                { return nil }
func (p *ptyHandle) Resize(cols, rows int) error { return nil }
func (p *ptyHandle) Wait() error                 { return nil }
