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
			id:          te.ID,
			parentID:    te.ParentID,
			startTime:   te.StartTime,
			ioStartTime: te.IOStartTime,
			current:     te.Current,
			total:       te.Total,
			name:        te.Name,
			depth:       depth,
			term:        vt100.NewVT100(6, 80),
			logTail:     newTail(32),
			progress:    p,
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
	ioStartTime        time.Time
	current, total     uint64
	displayRate        bool
	displayETA         bool
	displayBar         bool
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
	if te.IOStartTime != (time.Time{}) {
		t.ioStartTime = te.IOStartTime
	}
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
	if te.EnableDisplayRate {
		t.displayRate = true
	} else if te.DisableDisplayRate {
		t.displayRate = false
	}
	if te.EnableDisplayETA {
		t.displayETA = true
	} else if te.DisableDisplayETA {
		t.displayETA = false
	}
	if te.EnableDisplayBar {
		t.displayBar = true
	} else if te.DisableDisplayBar {
		t.displayBar = false
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
	arrow := mkarrow(t.depth)

	cached := ""
	if t.isCached {
		cached = " CACHED"
	}

	bytesCount := ""
	rate := ""
	eta := ""
	if t.current > 0 {
		bytesCount = fmt.Sprintf(" %.1f", units.Bytes(t.current))

		if t.total > 0 && !t.isDone {
			bytesCount = fmt.Sprintf("%s / %.1f", bytesCount, units.Bytes(t.total))
		}

		if t.total > 0 && !t.isDone && t.displayRate {
			rate = fmt.Sprintf(" (%.1f/s)", units.Bytes(float64(t.current)/time.Since(t.ioStartTime).Seconds()))
		}

		if t.total > 0 && !t.isDone && t.displayETA {
			bps := float64(t.current) / time.Since(t.ioStartTime).Seconds()
			secsRemain := float64(t.total-t.current) / bps
			etaDuration := time.Duration(secsRemain) * time.Second
			eta = fmt.Sprintf(" ETA %s", etaDuration)
		}
	}

	subtasks := ""
	if len(t.subtasks) > 0 {
		subtasks = fmt.Sprintf("(%d/%d)", t.subtasksDone, len(t.subtasks))
	}

	endTime := time.Now()
	if t.isDone {
		endTime = t.endTime
	}
	stopwatch := fmt.Sprintf("%.1fs", endTime.Sub(t.startTime).Seconds())

	left := fmt.Sprintf("%s%s %s%s%s%s", arrow, cached, t.name, bytesCount, rate, eta)
	right := fmt.Sprintf("%s %s", stopwatch, subtasks)

	if t.displayBar && t.total > 0 && !t.isDone {
		barLen := width - utf8.RuneCountInString(left) - utf8.RuneCountInString(right) - 2
		left = fmt.Sprintf("%s %s", left, mkbar(barLen, float64(t.current)/float64(t.total)))
	}

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
