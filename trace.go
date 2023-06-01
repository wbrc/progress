package progress

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/tonistiigi/units"
)

type knownTask struct {
	started time.Time
	name    string
}

type traceRenderer struct {
	name      string
	startTime time.Time

	knownTasks map[uint64]knownTask

	buf *bytes.Buffer
}

func (t *traceRenderer) update(te *TaskEvent) {
	secs := fmt.Sprintf("%.1f", time.Since(t.startTime).Seconds())
	header := fmt.Sprintf("[%5s]", secs)

	if task, ok := t.knownTasks[te.ID]; !ok {
		t.knownTasks[te.ID] = knownTask{
			started: te.StartTime,
			name:    te.Name,
		}

		fmt.Fprintf(t.buf, "%s START %q\n", header, te.Name)
	} else {
		if len(te.Logs) > 0 {
			logs, _ := bytes.CutSuffix(te.Logs, []byte("\n"))
			for _, line := range bytes.Split(logs, []byte("\n")) {
				fmt.Fprintf(t.buf, "%s %s: %s\n", header, t.name, string(line))
			}
		}

		if te.IsDone {
			secsDone := fmt.Sprintf("%.1f", time.Since(task.started).Seconds())

			var copied string
			if te.Current != 0 {
				copied = fmt.Sprintf("%.2f", units.Bytes(te.Current))
				if te.Total != 0 {
					copied = fmt.Sprintf("%s / %.2f", copied, units.Bytes(te.Total))
				}
				copied = fmt.Sprintf("(%s) ", copied)
			}

			var errStr string
			if te.HasErr {
				errStr = fmt.Sprintf(" with ERR %s", te.Err)
			}

			fmt.Fprintf(t.buf, "%s DONE %q %sin %ss%s\n", header, task.name, copied, secsDone, errStr)
		}
	}
}

func (t *traceRenderer) render(w io.Writer, _ int, _ bool) {
	if t.buf.Len() > 0 {
		_, _ = w.Write(t.buf.Bytes())
		t.buf.Reset()
	}
}

func newTraceRenderer(name string) *traceRenderer {
	return &traceRenderer{
		name:       name,
		startTime:  time.Now(),
		knownTasks: make(map[uint64]knownTask),
		buf:        bytes.NewBuffer(nil),
	}
}
