package parser

import (
	"bufio"
	"io"
)

// lineReader reads JSONL files line by line, skipping lines that
// exceed maxLen rather than aborting. The buffer starts small and
// grows on demand up to maxLen.
type lineReader struct {
	r      *bufio.Reader
	maxLen int
	buf    []byte
}

func newLineReader(r io.Reader, maxLen int) *lineReader {
	return &lineReader{
		r:      bufio.NewReaderSize(r, initialScanBufSize),
		maxLen: maxLen,
		buf:    make([]byte, 0, initialScanBufSize),
	}
}

// next returns the next line (without trailing newline) and true,
// or ("", false) at EOF. Lines exceeding maxLen are silently
// skipped.
func (lr *lineReader) next() (string, bool) {
	for {
		line, err := lr.readLine()
		if err != nil {
			return "", false
		}
		if line != "" {
			return line, true
		}
		// Empty line or skipped oversized line — continue.
	}
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
