package grid

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// CSVOptions controls CSV import / export. Defaults (CSVOptions{})
// produce RFC-4180-style output: comma delimiter, CRLF line endings,
// quote-when-necessary. Set Delimiter to ';' or '\t' for European CSV
// or TSV. AutoDetectDelimiter, on import only, sniffs the first non-
// empty line for the most frequent of {',', ';', '\t'} and uses that.
type CSVOptions struct {
	Delimiter           rune // default: ','
	LineEnd             string
	AutoDetectDelimiter bool
	IncludeHeader       bool // export header row; import treats first row as data
}

// SaveCSV writes the grid's data (after current filter/sort) to w.
// IncludeHeader controls whether a header row is emitted; default true
// is the convention SaveCSV-from-spreadsheet apps use.
func (g *StringGrid) SaveCSV(w io.Writer, opts CSVOptions) error {
	if opts.Delimiter == 0 {
		opts.Delimiter = ','
	}
	if opts.LineEnd == "" {
		opts.LineEnd = "\r\n"
	}
	bw := bufio.NewWriter(w)
	if opts.IncludeHeader {
		titles := make([]string, len(g.Columns))
		for i, c := range g.Columns {
			titles[i] = c.Title
		}
		if err := writeCSVRow(bw, titles, opts); err != nil {
			return err
		}
	}
	g.ensureVisible()
	for _, raw := range g.visibleRows {
		row := make([]string, len(g.Columns))
		for c := range g.Columns {
			row[c] = g.rawCell(raw, c)
		}
		if err := writeCSVRow(bw, row, opts); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func writeCSVRow(w *bufio.Writer, fields []string, opts CSVOptions) error {
	for i, f := range fields {
		if i > 0 {
			if _, err := w.WriteRune(opts.Delimiter); err != nil {
				return err
			}
		}
		if needsQuoting(f, opts.Delimiter) {
			if err := w.WriteByte('"'); err != nil {
				return err
			}
			for _, r := range f {
				if r == '"' {
					if _, err := w.WriteString(`""`); err != nil {
						return err
					}
				} else {
					if _, err := w.WriteRune(r); err != nil {
						return err
					}
				}
			}
			if err := w.WriteByte('"'); err != nil {
				return err
			}
		} else {
			if _, err := w.WriteString(f); err != nil {
				return err
			}
		}
	}
	_, err := w.WriteString(opts.LineEnd)
	return err
}

// needsQuoting reports whether f contains any of the characters that
// require quoting under RFC 4180 (delimiter, CR, LF, or quote itself).
func needsQuoting(f string, delim rune) bool {
	for _, r := range f {
		if r == delim || r == '"' || r == '\r' || r == '\n' {
			return true
		}
	}
	return false
}

// LoadCSV parses CSV from r and replaces the grid's rows. IncludeHeader
// in opts treats the first row as column titles (it's discarded from
// the data set, and column count is adjusted if needed).
func (g *StringGrid) LoadCSV(r io.Reader, opts CSVOptions) error {
	br := bufio.NewReader(r)
	if opts.AutoDetectDelimiter || opts.Delimiter == 0 {
		delim, peek, err := sniffDelimiter(br)
		if err != nil && err != io.EOF {
			return err
		}
		opts.Delimiter = delim
		// peek already consumed; we use a small pre-pended reader.
		br = bufio.NewReader(io.MultiReader(strings.NewReader(peek), br))
	}
	parsed, err := parseAllCSV(br, opts.Delimiter)
	if err != nil {
		return err
	}
	if len(parsed) == 0 {
		g.rows = nil
		g.markDirty()
		return nil
	}
	first := parsed[0]
	if opts.IncludeHeader {
		// Use the header row to populate Titles for any columns the
		// grid is missing. Existing column widths / aligns stay.
		for i, t := range first {
			if i >= len(g.Columns) {
				g.Columns = append(g.Columns, Column{Title: t, Width: 12, Align: AlignLeft, Sortable: true})
				g.Filters = append(g.Filters, "")
			} else if g.Columns[i].Title == "" {
				g.Columns[i].Title = t
			}
		}
		parsed = parsed[1:]
	}
	// Pad / clamp each row to match the column count.
	rows := make([][]string, len(parsed))
	for i, src := range parsed {
		row := make([]string, len(g.Columns))
		for j := 0; j < len(g.Columns); j++ {
			if j < len(src) {
				row[j] = src[j]
			}
		}
		rows[i] = row
	}
	g.rows = rows
	g.markDirty()
	g.Top = 0
	g.Focus, g.Anchor = Cell{}, Cell{}
	return nil
}

// sniffDelimiter peeks at the first line of the reader and returns
// whichever of ',', ';', '\t' appears most often. Returns ',' as a
// safe default when there are no delimiters.
func sniffDelimiter(br *bufio.Reader) (rune, string, error) {
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return ',', line, err
	}
	best := rune(',')
	bestN := 0
	for _, d := range []rune{',', ';', '\t'} {
		n := 0
		inQuote := false
		for _, c := range line {
			if c == '"' {
				inQuote = !inQuote
			}
			if !inQuote && c == d {
				n++
			}
		}
		if n > bestN {
			bestN = n
			best = d
		}
	}
	return best, line, err
}

// parseAllCSV reads the entire stream and returns the parsed rows.
// Implements RFC 4180: quoted fields preserve their delimiter / CR /
// LF; doubled quotes inside a quoted field are a literal quote;
// unquoted fields end at the next delimiter or line ending.
func parseAllCSV(r *bufio.Reader, delim rune) ([][]string, error) {
	if delim == 0 {
		delim = ','
	}
	var out [][]string
	var row []string
	var field strings.Builder
	inQuote := false
	for {
		c, _, err := r.ReadRune()
		if err == io.EOF {
			if field.Len() > 0 || len(row) > 0 {
				row = append(row, field.String())
				out = append(out, row)
			}
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if inQuote {
			if c == '"' {
				next, _, e := r.ReadRune()
				if e == io.EOF {
					inQuote = false
					continue
				}
				if e != nil {
					return out, e
				}
				if next == '"' {
					field.WriteRune('"')
					continue
				}
				if err := r.UnreadRune(); err != nil {
					return out, err
				}
				inQuote = false
				continue
			}
			field.WriteRune(c)
			continue
		}
		switch c {
		case '"':
			inQuote = true
		case delim:
			row = append(row, field.String())
			field.Reset()
		case '\r':
			// Look for the optional LF.
			next, _, e := r.ReadRune()
			if e == nil && next != '\n' {
				if err := r.UnreadRune(); err != nil {
					return out, err
				}
			}
			row = append(row, field.String())
			field.Reset()
			out = append(out, row)
			row = nil
		case '\n':
			row = append(row, field.String())
			field.Reset()
			out = append(out, row)
			row = nil
		default:
			field.WriteRune(c)
		}
	}
}

// ensure the io.Reader path compiles even if a future refactor moves
// these out.
var _ error = errors.New("placeholder")
