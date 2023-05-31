package progress

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/containerd/console"
	"github.com/morikuni/aec"
	"github.com/tonistiigi/units"
	"github.com/tonistiigi/vt100"
	"golang.org/x/time/rate"
)

func MainLoop(c console.Console, name string, events <-chan *TaskEvent) {
	p := &progress{
		name:      name,
		startTime: time.Now(),
		allTasks:  make(map[uint64]*task),
	}

	tickRate := 150 * time.Millisecond
	rateLimit := 100 * time.Millisecond

	t := time.NewTicker(tickRate)
	r := rate.NewLimiter(rate.Every(rateLimit), 1)

	for done := false; !done; {
		select {
		case <-t.C:
		case e, ok := <-events:
			if !ok {
				done = true
			} else {
				updateProgress(p, e)
			}
		}

		if done || r.Allow() {
			size, err := c.Size()
			if err != nil {
				size = console.WinSize{Width: 80}
			}
			p.render(c, int(size.Width), done)
			t.Stop()
			t = time.NewTicker(tickRate)
		}
	}
}

type TaskEvent struct {
	ID       uint64
	ParentID uint64

	Name string

	StartTime, EndTime time.Time
	IsDone             bool

	Current, Total uint64

	HasErr bool
	Err    error

	Logs []byte
}

func updateProgress(p *progress, te *TaskEvent) {
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

type progress struct {
	name      string
	startTime time.Time
	tasks     []*task
	allTasks  map[uint64]*task
	tasksDone int
	lines     int
}

func (p *progress) render(w io.Writer, width int, showError bool) {
	fmt.Fprint(w, aec.Up(uint(p.lines)))

	left := fmt.Sprintf("+ %s", p.name)
	right := fmt.Sprintf("(%d/%d) %3.1fs", p.tasksDone, len(p.tasks), time.Since(p.startTime).Seconds())
	titleLine := align(left, right, width)

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
}

type task struct {
	id                 uint64
	parentID           uint64
	name               string
	depth              int
	startTime, endTime time.Time
	current, total     uint64
	isDone             bool
	hasError           bool
	err                error
	logs               [][]byte
	term               *vt100.VT100
	subtasks           []*task
	subtasksDone       int
	progress           *progress
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

	if te.HasErr {
		t.hasError = true
		t.err = te.Err
	}

	if len(te.Logs) > 0 {
		t.logs = append(t.logs, te.Logs)
	}
}

func (t *task) render(w io.Writer, width int, showError bool) int {
	left := fmt.Sprintf("%s %s", arrow(t.depth), t.name)

	if t.total > 0 {
		left = fmt.Sprintf("%s %.2f / %.2f", left, units.Bytes(t.current), units.Bytes(t.total))
	} else if t.current > 0 {
		left = fmt.Sprintf("%s %.2f", left, units.Bytes(t.current))
	}

	if t.hasError {
		left += " [ERROR]"
	}

	right := ""
	if len(t.subtasks) > 0 {
		right = fmt.Sprintf("(%d/%d)", t.subtasksDone, len(t.subtasks))
	}

	endTime := time.Now()
	if t.isDone {
		endTime = t.endTime
	}
	right = fmt.Sprintf("%s %3.1fs", right, endTime.Sub(t.startTime).Seconds())

	titleLine := align(left, right, width)
	if t.hasError {
		titleLine = aec.Apply(titleLine, aec.RedF, aec.Bold)
	} else if t.isDone {
		titleLine = aec.Apply(titleLine, aec.BlueF)
	}

	fmt.Fprintln(w, titleLine)
	lines := 1

	if !t.isDone {
		t.term.Resize(6, width)
		_, _ = t.term.Write(merge(t.logs))
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
