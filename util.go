package progress

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/tonistiigi/vt100"
)

func renderTerm(term *vt100.VT100, w io.Writer) {
	h := term.UsedHeight()
	buf := &bytes.Buffer{}
	for _, line := range term.Content[:h] {
		for _, cell := range line {
			buf.WriteRune(cell)
		}
		buf.WriteRune('\n')
	}
	_, _ = w.Write(buf.Bytes()) // explicitly ignore any errors
}

func align(l, r string, w int) string {
	return fmt.Sprintf("%-[2]*[1]s %[3]s", l, w-len(r)-1, r)
}

func arrow(len int) string {
	return strings.TrimSuffix(strings.Repeat("=> ", len), " ")
}

func bar(width int, percent float64) string {
	if width < 3 {
		return ""
	}

	width -= 2

	sb := &strings.Builder{}
	sb.Grow(width + 2)

	sb.WriteRune('[')
	for i := 0; i < width; i++ {
		if float64(i) < percent*float64(width) {
			sb.WriteRune('=')
		} else {
			sb.WriteRune(' ')
		}
	}
	sb.WriteRune(']')

	return sb.String()
}

func merge(bufs [][]byte) []byte {
	var buf bytes.Buffer
	for _, line := range bufs {
		buf.Write(line)
	}
	return buf.Bytes()
}

type countReader struct {
	notify func(int64)
	r      io.Reader
	n      int64
}

func (r *countReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.n += int64(n)
	r.notify(r.n)
	return n, err
}
