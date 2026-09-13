package transcript

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// Line is one complete transcript line, or an error encountered while reading.
type Line struct {
	Text string
	Err  error
}

// Tail emits every complete line already in the file, then polls the file
// every poll interval and emits lines as they are completed by a trailing
// newline. A partial line is held until its newline arrives. If the file
// shrinks, reading restarts from the beginning. The channel closes when ctx
// is cancelled. An error opening the file is sent once and the channel closes.
// maxLine bounds the bytes held for one unterminated line; a longer one is
// dropped with an error and reading resumes at the next newline. Same bound
// as Recent's scanner. A variable so tests can lower it.
var maxLine = 64 << 20

func Tail(ctx context.Context, path string, poll time.Duration) <-chan Line {
	out := make(chan Line)
	go func() {
		defer close(out)
		f, err := os.Open(path)
		if err != nil {
			send(ctx, out, Line{Err: err})
			return
		}
		defer f.Close()
		var (
			offset   int64
			partial  []byte
			dropping bool // inside a line that exceeded maxLine: skip to its newline
			buf      = make([]byte, 64*1024)
		)
		for {
			// Read everything available from offset.
			for {
				n, rerr := f.ReadAt(buf, offset)
				if n > 0 {
					offset += int64(n)
					partial = append(partial, buf[:n]...)
					for {
						i := bytes.IndexByte(partial, '\n')
						if i < 0 {
							if len(partial) > maxLine {
								partial, dropping = partial[:0], true
								if !send(ctx, out, Line{Err: fmt.Errorf("line longer than %d bytes dropped", maxLine)}) {
									return
								}
							}
							break
						}
						if dropping { // the tail of the dropped line
							partial, dropping = partial[i+1:], false
							continue
						}
						if i > maxLine { // a whole over-long line arrived in one read
							partial = partial[i+1:]
							if !send(ctx, out, Line{Err: fmt.Errorf("line longer than %d bytes dropped", maxLine)}) {
								return
							}
							continue
						}
						text := string(partial[:i])
						partial = partial[i+1:]
						if !send(ctx, out, Line{Text: text}) {
							return
						}
					}
				}
				if rerr == io.EOF {
					break
				}
				if rerr != nil {
					if !send(ctx, out, Line{Err: rerr}) {
						return
					}
					break
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(poll):
			}
			st, serr := f.Stat()
			if serr != nil {
				if !send(ctx, out, Line{Err: serr}) {
					return
				}
				continue
			}
			if st.Size() < offset { // truncated: start over
				offset = 0
				partial, dropping = partial[:0], false
			}
		}
	}()
	return out
}

func send(ctx context.Context, out chan<- Line, l Line) bool {
	select {
	case out <- l:
		return true
	case <-ctx.Done():
		return false
	}
}
