package parser

import (
	"bufio"
	"io"
)

// lineReader reads JSONL files line by line, skipping lines that
// exceed maxLen rather than aborting. The buffer starts small and
// grows on demand up to maxLen. After iteration, call Err() to
// check for I/O errors (as opposed to normal EOF).
type lineReader struct {
	r      *bufio.Reader
	maxLen int
	buf    []byte
	err    error
}

func newLineReader(r io.Reader, maxLen int) *lineReader {
	return &lineReader{
		r:      bufio.NewReaderSize(r, initialScanBufSize),
		maxLen: maxLen,
		buf:    make([]byte, 0, initialScanBufSize),
	}
}

// next returns the next line (without trailing newline) and true,
// or ("", false) at EOF or read error. After the loop, call Err()
// to distinguish EOF from I/O failure.
func (lr *lineReader) next() (string, bool) {
	for {
		line, err := lr.readLine()
		if err != nil {
			if err != io.EOF {
				lr.err = err
			}
			return "", false
		}
		if line != "" {
			return line, true
		}
		// Empty line or skipped oversized line — continue.
	}
}

// Err returns the first non-EOF read error encountered, or nil.
func (lr *lineReader) Err() error {
	return lr.err
}

// readLine reads a full line, returning "" for blank/oversized
// lines and a non-nil error only at EOF or read failure.
func (lr *lineReader) readLine() (string, error) {
	lr.buf = lr.buf[:0]
	oversized := false

	for {
		chunk, isPrefix, err := lr.r.ReadLine()
		if err != nil {
			if len(lr.buf) > 0 && err == io.EOF {
				break
			}
			return "", err
		}

		if oversized {
			if !isPrefix {
				return "", nil // done skipping
			}
			continue
		}

		lr.buf = append(lr.buf, chunk...)

		if len(lr.buf) > lr.maxLen {
			oversized = true
			lr.buf = lr.buf[:0]
			if !isPrefix {
				return "", nil
			}
			continue
		}

		if !isPrefix {
			break
		}
	}

	return string(lr.buf), nil
}
