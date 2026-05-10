package syntax

import "github.com/oldwired/fv-go/pkg/fv/widgets/editor"

// ToEditorColorer wraps h so it satisfies editor.LineColorer. The
// editor package can't import syntax (it would invert the natural
// layering), so the adapter lives here.
func (h *Highlighter) ToEditorColorer() editor.LineColorer {
	return editorAdapter{h}
}

type editorAdapter struct{ h *Highlighter }

func (a editorAdapter) Tokenize(line string) []editor.ColorSpan {
	src := a.h.Tokenize(line)
	out := make([]editor.ColorSpan, len(src))
	for i, s := range src {
		out[i] = editor.ColorSpan{Start: s.Start, End: s.End, Attr: s.Attr}
	}
	return out
}
