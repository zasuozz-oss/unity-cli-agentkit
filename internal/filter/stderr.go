package filter

import (
	"bytes"
	"io"
	"strings"
)

// noiseTag marks the official CLI's telemetry failures. On a machine with no
// proxy it fails on every `unity test` invocation and dumps ~2.5KB of
// ECONNREFUSED stack traces — more output than the test run itself produces.
const noiseTag = "[Experiment]"

// NoiseFilter wraps a stderr writer and drops the telemetry blocks: the tagged
// line plus the indented stack that follows it, up to its closing bracket.
// Every other byte passes through, so real diagnostics are never swallowed.
type NoiseFilter struct {
	W       io.Writer
	buf     []byte
	inNoise bool
}

func (f *NoiseFilter) Write(p []byte) (int, error) {
	f.buf = append(f.buf, p...)
	for {
		i := bytes.IndexByte(f.buf, '\n')
		if i < 0 {
			break
		}
		line := f.buf[:i+1]
		f.buf = f.buf[i+1:]
		if err := f.line(line); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Close flushes a trailing partial line, so output that never ends in a
// newline is not silently dropped.
func (f *NoiseFilter) Close() error {
	if len(f.buf) == 0 {
		return nil
	}
	line := f.buf
	f.buf = nil
	return f.line(line)
}

func (f *NoiseFilter) line(line []byte) error {
	s := string(line)
	if f.inNoise {
		// The block ends at the first line that is neither indented nor the
		// bracket closing the error array.
		if t := strings.TrimRight(s, "\r\n"); t == "]" {
			f.inNoise = false
			return nil
		} else if t == "" || strings.HasPrefix(t, " ") || strings.HasPrefix(t, "\t") {
			return nil
		}
		f.inNoise = false // ran past the block; fall through and print it
	}
	if strings.HasPrefix(s, noiseTag) {
		f.inNoise = true
		return nil
	}
	_, err := f.W.Write(line)
	return err
}
