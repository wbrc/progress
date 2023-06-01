package progress

import (
	"bytes"
	"container/list"
	"io"
)

type tail struct {
	lines *list.List
	cap   int
}

func newTail(maxLines int) *tail {
	return &tail{
		cap:   maxLines,
		lines: list.New(),
	}
}

func (t *tail) Len() int {
	return t.lines.Len()
}

func (t *tail) Write(p []byte) (int, error) {
	lines := bytes.Split(p, []byte("\n"))
	for lineIdx, line := range lines {
		clone := make([]byte, len(line))
		copy(clone, line)

		if lineIdx == len(lines)-1 && len(clone) == 0 {
			continue
		}

		t.lines.PushBack(clone)
		if t.lines.Len() > t.cap {
			t.lines.Remove(t.lines.Front())
		}
	}
	return len(p), nil
}

func (t *tail) writeTo(w io.Writer) {
	for e := t.lines.Front(); e != nil; e = e.Next() {
		_, _ = w.Write(e.Value.([]byte))
		_, _ = w.Write([]byte("\n"))
	}
}
