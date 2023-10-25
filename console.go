package progress

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/morikuni/aec"
	"github.com/tonistiigi/units"
	"github.com/tonistiigi/vt100"
)

type consoleRenderer struct {
	name      string
	startTime time.Time
	tasks     []*task
	allTasks  map[uint64]*task
	tasksDone int
	lines     int
	hasError  bool
}

func (p *consoleRenderer) update(te *TaskEvent) {
	if te.ID == 0 {
		return
	}

	if existingTask, ok := p.allTasks[te.ID]; ok {
		existingTask.update(te)
	} else {
		parent, hasParent := p.allTasks[te.ParentID]

		depth := 1
		if hasParent {
			depth = parent.depth + 1
		}

		newTask := &task{
			id:        te.ID,
			parentID:  te.ParentID,
			startTime: te.StartTime,
			current:   te.Current,
			total:     te.Total,
			name:      te.Name,
			depth:     depth,
			term:      vt100.NewVT100(6, 80),
			logTail:   newTail(32),
			progress:  p,
		}

		p.allTasks[te.ID] = newTask
		if hasParent {
			parent.subtasks = append(parent.subtasks, newTask)
		} else {
			p.tasks = append(p.tasks, newTask)
		}
	}
}

func (p *consoleRenderer) render(w io.Writer, width int, showError bool) {
	fmt.Fprint(w, aec.Up(uint(p.lines)))

	left := fmt.Sprintf("+ %s", p.name)
	right := fmt.Sprintf("(%d/%d) %.1fs", p.tasksDone, len(p.tasks), time.Since(p.startTime).Seconds())
	titleLine := align(left, right, width)

	if showError {
		if p.hasError {
			titleLine = aec.Apply(titleLine, aec.RedF, aec.Bold)
		} else {
			titleLine = aec.Apply(titleLine, aec.BlueF)
		}
	}

	fmt.Fprintln(w, titleLine)
	lineCnt := 1
	for _, task := range p.tasks {
		lineCnt += task.render(w, width, showError)
	}

	if diff := p.lines - lineCnt; diff > 0 {
		for i := 0; i < diff; i++ {
			fmt.Fprintln(w, strings.Repeat(" ", width))
		}
		fmt.Fprint(w, aec.Up(uint(diff)))
	}
	p.lines = lineCnt

	if showError {
		for _, task := range p.tasks {
			task.renderLogs(w)
		}
	}
}

func newConsoleRenderer(name string) *consoleRenderer {
	return &consoleRenderer{
		name:      name,
		startTime: time.Now(),
		allTasks:  make(map[uint64]*task),
	}
}

type task struct {
	id                 uint64
	parentID           uint64
	name               string
	depth              int
	startTime, endTime time.Time
	current, total     uint64
	isDone             bool
	isCached           bool
	hasError           bool
	err                error
	logs               [][]byte
	term               *vt100.VT100
	logTail            *tail
	subtasks           []*task
	subtasksDone       int
	progress           *consoleRenderer
}

func (t *task) update(te *TaskEvent) {
	if te.Name != "" {
		t.name = te.Name
	}
	t.endTime = te.EndTime
	if te.Current > 0 {
		t.current = te.Current
	}
	if te.Total > 0 {
		t.total = te.Total
	}
	if !t.isDone && te.IsDone {
		t.isDone = true
		if parent, ok := t.progress.allTasks[t.parentID]; ok {
			parent.subtasksDone++
		} else {
			t.progress.tasksDone++
		}
	}
	if te.Cached {
		t.isCached = true
	}

	if te.HasErr {
		t.hasError = true
		t.err = te.Err
		t.progress.hasError = true
	}

	if len(te.Logs) > 0 {
		t.logs = append(t.logs, te.Logs)
	}
}

func (t *task) render(w io.Writer, width int, showError bool) int {
	cached := ""
	if t.isCached {
		cached = "CACHED "
	}
	left := fmt.Sprintf("%s %s%s", arrow(t.depth), cached, t.name)

	if t.current > 0 {
		current := fmt.Sprintf(" %.1f", units.Bytes(t.current))

		var total string
		if t.total > 0 && !t.isDone {
			total = fmt.Sprintf(" / %.1f", units.Bytes(t.total))
		}

		left = fmt.Sprintf("%s%s%s", left, current, total)
	}

	right := ""
	if len(t.subtasks) > 0 {
		right = fmt.Sprintf("(%d/%d)", t.subtasksDone, len(t.subtasks))
	}

	endTime := time.Now()
	if t.isDone {
		endTime = t.endTime
	}
	right = fmt.Sprintf("%s %.1fs", right, endTime.Sub(t.startTime).Seconds())

	titleLine := align(left, right, width)
	if t.hasError {
		titleLine = aec.Apply(titleLine, aec.RedF, aec.Bold)
	} else if t.isDone {
		titleLine = aec.Apply(titleLine, aec.BlueF)
		if t.isCached {
			titleLine = aec.Apply(titleLine, aec.Bold)
		}
	}

	fmt.Fprintln(w, titleLine)
	lines := 1

	if !t.isDone {
		t.term.Resize(6, width)
		mergedLogs := merge(t.logs)
		_, _ = t.term.Write(mergedLogs)
		_, _ = t.logTail.Write(mergedLogs)
		t.logs = nil

		buf := &bytes.Buffer{}
		renderTerm(t.term, buf)
		fmt.Fprint(w, aec.Apply(buf.String(), aec.Faint))
		lines += t.term.UsedHeight()
	}

	if showError && t.hasError {
		t.term = vt100.NewVT100(6, width)
		fmt.Fprintln(t.term, t.err)
		buf := &bytes.Buffer{}
		renderTerm(t.term, buf)
		fmt.Fprint(w, aec.Apply(buf.String(), aec.RedF))
		lines += t.term.UsedHeight()
	}

	for _, subtask := range t.subtasks {
		lines += subtask.render(w, width, showError)
	}

	return lines
}

func (t *task) renderLogs(w io.Writer) {
	if t.logTail.Len() > 0 && t.hasError {
		logBuf := &bytes.Buffer{}
		header := fmt.Sprintf("=== LOG DUMP %s ===", t.name)
		fmt.Fprintln(logBuf, header)
		t.logTail.writeTo(logBuf)
		fmt.Fprintln(logBuf, strings.Repeat("=", utf8.RuneCountInString(header)))
		fmt.Fprint(w, aec.Apply(logBuf.String(), aec.Faint))
	}

	for _, subtask := range t.subtasks {
		subtask.renderLogs(w)
	}
}
