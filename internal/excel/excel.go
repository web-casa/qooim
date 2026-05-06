// Package excel wraps xuri/excelize for the import/export flows. We use
// the streaming writer when emitting xlsx so that large answer exports
// don't load all rows into memory.
package excel

import (
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"
)

// Writer streams rows to an xlsx in memory, then ships them through
// excelize's StreamWriter so peak memory stays bounded.
type Writer struct {
	f      *excelize.File
	stream *excelize.StreamWriter
	row    int
}

// NewWriter returns a Writer that emits to the default sheet.
// `headers` is the first row.
func NewWriter(headers []string) (*Writer, error) {
	f := excelize.NewFile()
	sw, err := f.NewStreamWriter("Sheet1")
	if err != nil {
		return nil, fmt.Errorf("stream writer: %w", err)
	}
	row := make([]any, len(headers))
	for i, h := range headers {
		row[i] = h
	}
	if err := sw.SetRow("A1", row); err != nil {
		return nil, fmt.Errorf("write headers: %w", err)
	}
	return &Writer{f: f, stream: sw, row: 1}, nil
}

// AppendRow writes one row of arbitrary scalar values.
func (w *Writer) AppendRow(values []any) error {
	w.row++
	cell, err := excelize.CoordinatesToCellName(1, w.row)
	if err != nil {
		return err
	}
	return w.stream.SetRow(cell, values)
}

// Flush finalises the stream and writes the workbook to dst. The
// underlying excelize file is always closed before this method returns,
// even on a partial write.
func (w *Writer) Flush(dst io.Writer) error {
	defer func() { _ = w.f.Close() }()
	if err := w.stream.Flush(); err != nil {
		return fmt.Errorf("flush stream: %w", err)
	}
	if err := w.f.Write(dst); err != nil {
		return fmt.Errorf("write xlsx: %w", err)
	}
	return nil
}

// Close releases the workbook without writing it. Safe to call after
// Flush — excelize.Close is idempotent enough for our needs.
func (w *Writer) Close() error { return w.f.Close() }

// ReadAllRows opens an xlsx from r and returns every non-empty row of
// the first sheet. Header row is included as rows[0].
// Used by the template-import flow, which pre-allocates the whole file
// in memory — fine for the input sizes we expect (a few thousand rows).
func ReadAllRows(r io.Reader) ([][]string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets in workbook")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("read rows: %w", err)
	}
	return rows, nil
}
