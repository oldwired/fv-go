package grid

import (
	"bufio"
	"encoding/csv"
	"io"
	"strings"
)

// CSVOptions controls CSV import / export. Zero value is minimal and
// boring (`CSVOptions{}` → comma, LF, no header). Use DefaultCSVOptions
// for spreadsheet-style output (CRLF + header). AutoDetectDelimiter,
// on import only, sniffs the first non-empty line for the most frequent
// of {',', ';', '\t'} and overrides Delimiter.
type CSVOptions struct {
	Delimiter           rune   // zero → ','
	LineEnd             string // zero → "\n"
	AutoDetectDelimiter bool
	IncludeHeader       bool // export header row; import treats first row as data
}

// DefaultCSVOptions returns the spreadsheet-style configuration:
// comma delimiter, CRLF line endings, header on export / import.
func DefaultCSVOptions() CSVOptions {
	return CSVOptions{
		Delimiter:     ',',
		LineEnd:       "\r\n",
		IncludeHeader: true,
	}
}

// normalizeCSVOptions fills in zero-value Delimiter / LineEnd so the
// rest of the code doesn't have to defensive-check.
func normalizeCSVOptions(opts CSVOptions) CSVOptions {
	if opts.Delimiter == 0 {
		opts.Delimiter = ','
	}
	if opts.LineEnd == "" {
		opts.LineEnd = "\n"
	}
	return opts
}

// SaveCSV writes the grid's data to w using encoding/csv. The export
// reflects the current filter/sort (visible rows in their displayed
// order) and includes all columns — Column.Hidden is visual state, not
// a data filter.
//
// LineEnd only respects "\n" vs "\r\n"; encoding/csv.Writer doesn't
// expose finer line-ending control. Other values are interpreted as
// CRLF if they contain '\r', else LF.
func (g *StringGrid) SaveCSV(w io.Writer, opts CSVOptions) error {
	opts = normalizeCSVOptions(opts)
	cw := csv.NewWriter(w)
	cw.Comma = opts.Delimiter
	cw.UseCRLF = strings.Contains(opts.LineEnd, "\r")
	if opts.IncludeHeader {
		titles := make([]string, len(g.Columns))
		for i, c := range g.Columns {
			titles[i] = c.Title
		}
		if err := cw.Write(titles); err != nil {
			return err
		}
	}
	g.ensureVisible()
	row := make([]string, len(g.Columns))
	for _, raw := range g.visibleRows {
		for c := range g.Columns {
			row[c] = g.rawCell(raw, c)
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// LoadCSV parses CSV from r and replaces the grid's rows. Uses
// encoding/csv so malformed input (notably EOF inside a quoted field)
// surfaces as an error — the previous hand-rolled parser silently
// accepted truncated quotes.
func (g *StringGrid) LoadCSV(r io.Reader, opts CSVOptions) error {
	br := bufio.NewReader(r)
	if opts.AutoDetectDelimiter || opts.Delimiter == 0 {
		delim, peek, err := sniffDelimiter(br)
		if err != nil && err != io.EOF {
			return err
		}
		opts.Delimiter = delim
		br = bufio.NewReader(io.MultiReader(strings.NewReader(peek), br))
	}
	opts = normalizeCSVOptions(opts)
	cr := csv.NewReader(br)
	cr.Comma = opts.Delimiter
	cr.FieldsPerRecord = -1 // tolerate ragged rows
	parsed, err := cr.ReadAll()
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
