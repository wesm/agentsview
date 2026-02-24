package parser

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLineReader(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   []string
	}{
		{
			"normal lines",
			"aaa\nbbb\nccc\n",
			100,
			[]string{"aaa", "bbb", "ccc"},
		},
		{
			"skips oversized line",
			"short\n" + strings.Repeat("x", 50) + "\nafter\n",
			30,
			[]string{"short", "after"},
		},
		{
			"all lines oversized",
			strings.Repeat("a", 50) + "\n" +
				strings.Repeat("b", 50) + "\n",
			30,
			nil,
		},
		{
			"empty input",
			"",
			100,
			nil,
		},
		{
			"blank lines skipped",
			"aaa\n\n\nbbb\n",
			100,
			[]string{"aaa", "bbb"},
		},
		{
			"line without trailing newline",
			"aaa\nbbb",
			100,
			[]string{"aaa", "bbb"},
		},
		{
			"exact limit kept",
			strings.Repeat("x", 30) + "\n",
			30,
			[]string{strings.Repeat("x", 30)},
		},
		{
			"one over limit skipped",
			strings.Repeat("x", 31) + "\n",
			30,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lr := newLineReader(
				strings.NewReader(tt.input), tt.maxLen,
			)
			var got []string
			for {
				line, ok := lr.next()
				if !ok {
					break
				}
				got = append(got, line)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d lines, want %d: %v",
					len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line[%d] = %q, want %q",
						i, got[i], tt.want[i])
				}
			}
		})
	}
}

// errAfterReader yields data from buf, then returns errIO on the
// next read.
type errAfterReader struct {
	buf   *strings.Reader
	errIO error
	done  bool
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.errIO
	}
	n, err := r.buf.Read(p)
	if err == io.EOF {
		r.done = true
		return n, r.errIO
	}
	return n, err
}

func TestLineReaderIOError(t *testing.T) {
	ioErr := errors.New("disk read failed")
	r := &errAfterReader{
		buf:   strings.NewReader("aaa\nbbb\n"),
		errIO: ioErr,
	}

	lr := newLineReader(r, 100)
	var got []string
	for {
		line, ok := lr.next()
		if !ok {
			break
		}
		got = append(got, line)
	}

	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(got), got)
	}
	if lr.Err() == nil {
		t.Fatal("expected non-nil Err() after I/O failure")
	}
	if !errors.Is(lr.Err(), ioErr) {
		t.Fatalf("Err() = %v, want %v", lr.Err(), ioErr)
	}
}
