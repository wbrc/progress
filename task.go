package progress

import (
	"io"
	"time"

	"github.com/containerd/console"
)

// RootTask is a task that can be used to close the channel of events.
type RootTask struct {
	Task
}

// Close closes the channel of events.
func (r *RootTask) Close() error {
	close(r.ch)
	return nil
}

// NewRootTask creates a new RootTask that sends events to the given channel.
func NewRootTask(ch chan *TaskEvent) *RootTask {
	return &RootTask{
		Task{
			ch: ch,
		},
	}
}

// DisplayProgress displays progress events to the console or trace. It is
// a convenience function that creates a RootTask and returns a channel that
// is closed when the rendering is complete. The caller has to make sure to
// close the RootTask after all Subtasks are completed. After the RootTask is
// closed, the remaining unprocesses events are rendered and the returned
// channel is closed.
func DisplayProgress(f console.File, name, mode string) (*RootTask, <-chan struct{}, error) {
	events := make(chan *TaskEvent)

	done, err := ProcessEvents(f, name, mode, events)
	if err != nil {
		return nil, nil, err
	}

	return NewRootTask(events), done, nil
}

// TaskEvent carries all the information about tasks. You'll only need this if
// you do not want to use the Task interfaces and provide the events yourself.
type TaskEvent struct {
	ID       uint64 // unique ID for the task, must be > 0
	ParentID uint64 // ID of the parent task, 0 if no parent

	Name string // name of the task, this will be displayed in the header line

	StartTime, EndTime time.Time // start and end time of the task, used to calculate the duration
	IOStartTime        time.Time // start time of the IO task, used to calculate the rate and ETA
	IsDone             bool      // true if the task is done, finished tasks will be displayed differently

	Cached bool // true if the task is cached, cached tasks will be displayed differently when they are done

	// Current and Total are used to display a copy progress if Total is
	// unknown leave it as 0 and only Current will be displayed
	Current, Total uint64

	EnableDisplayRate  bool // true if displaying the rate should be enabled, only used for io tasks
	DisableDisplayRate bool // true if displaying the rate should be disabled, only used for io tasks
	EnableDisplayBar   bool // true if displaying the bar should be enabled, only used for io tasks

	EnableDisplayETA  bool // true if displaying the ETA should be enabled, only used for io tasks
	DisableDisplayETA bool // true if displaying the ETA should be disabled, only used for io tasks
	DisableDisplayBar bool // true if displaying the bar should be disabled, only used for io tasks

	HasErr bool  // true if the task has an error
	Err    error // error of the task, will be displayed in the task body when all tasks are done

	Logs []byte // logs of the task, will be displayed in the task body
}

// TaskLogger implements io.Writer and writes logs to the task.
type TaskLogger struct {
	ch chan *TaskEvent
	id uint64
}

// Write writes logs to the task.
func (l *TaskLogger) Write(p []byte) (int, error) {
	pp := make([]byte, len(p))
	copy(pp, p)
	l.ch <- &TaskEvent{
		ID:   l.id,
		Logs: pp,
	}
	return len(p), nil
}

// Task is the base type for all tasks. It provides the basic functionality
// for tasks like logging and launching subtasks.
type Task struct {
	id uint64
	ch chan *TaskEvent
}

// Logger returns a *TaskLogger that can be used to write logs to the task.
func (t *Task) Logger() *TaskLogger {
	return &TaskLogger{t.ch, t.id}
}

// Name sets the name of the task.
func (t *Task) Name(name string) {
	t.ch <- &TaskEvent{
		ID:   t.id,
		Name: name,
	}
}

// Execute launches a new subtask by calling the given function and waits for
// it to complete. If f returns an error, the task will be marked as failed and
// the error will be returned.
func (t *Task) Execute(name string, f func(*Task) error) error {
	newID := uint64(time.Now().UnixNano())
	now := time.Now()
	t.ch <- &TaskEvent{
		ID:          newID,
		ParentID:    t.id,
		Name:        name,
		StartTime:   now,
		IOStartTime: now,
	}

	err := f(&Task{newID, t.ch})

	t.ch <- &TaskEvent{
		ID:      newID,
		EndTime: time.Now(),
		IsDone:  true,
		HasErr:  err != nil,
		Err:     err,
	}

	return err
}

// Cached marks the task as cached.
func (t *Task) Cached() {
	t.ch <- &TaskEvent{
		ID:     t.id,
		Cached: true,
	}
}

// IOTask is a task that can be used to display IO progress.
type IOTask struct {
	Task
}

// DisplayRate enables or disables the display of the rate.
func (t *IOTask) DisplayRate(b bool) {
	EnableDisplayRate := b
	DisableDisplayRate := !b
	t.ch <- &TaskEvent{
		ID:                 t.id,
		EnableDisplayRate:  EnableDisplayRate,
		DisableDisplayRate: DisableDisplayRate,
	}
}

// DisplayETA enables or disables the display of the ETA.
func (t *IOTask) DisplayETA(b bool) {
	EnableDisplayETA := b
	DisableDisplayETA := !b
	t.ch <- &TaskEvent{
		ID:                t.id,
		EnableDisplayETA:  EnableDisplayETA,
		DisableDisplayETA: DisableDisplayETA,
	}
}

// DisplayBar enables or disables the display of a progress bar.
func (t *IOTask) DisplayBar(b bool) {
	EnableDisplayBar := b
	DisableDisplayBar := !b
	t.ch <- &TaskEvent{
		ID:                t.id,
		EnableDisplayBar:  EnableDisplayBar,
		DisableDisplayBar: DisableDisplayBar,
	}
}

// ReaderTask tracks the progress of reading from an underlying io.Reader
type ReaderTask struct {
	IOTask
	read uint64
	r    io.Reader
}

// Read reads from the underlying reader and updates the progress.
func (t *ReaderTask) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)

	t.read += uint64(n)
	t.ch <- &TaskEvent{
		ID:      t.id,
		Current: t.read,
	}

	return n, err
}

// Reader launches a new subtask that reads from the given reader. If total is
// 0, the task will not display a progress bar or ETA.
func (t *Task) Reader(name string, r io.Reader, total uint64, f func(*ReaderTask) error) error {
	newID := uint64(time.Now().UnixNano())
	now := time.Now()
	t.ch <- &TaskEvent{
		ID:          newID,
		ParentID:    t.id,
		Name:        name,
		Total:       total,
		StartTime:   now,
		IOStartTime: now,
	}

	rt := &ReaderTask{IOTask{Task{newID, t.ch}}, 0, r}

	err := f(rt)

	t.ch <- &TaskEvent{
		ID:      newID,
		EndTime: time.Now(),
		Current: rt.read,
		IsDone:  true,
		HasErr:  err != nil,
		Err:     err,
	}
	return err
}

// WriterTask tracks the progress of writing to an underlying io.Writer
type WriterTask struct {
	IOTask
	written uint64
	w       io.Writer
}

// Write writes to the underlying writer and updates the progress.
func (t *WriterTask) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)

	t.written += uint64(n)
	t.ch <- &TaskEvent{
		ID:      t.id,
		Current: t.written,
	}

	return n, err
}

// Writer launches a new subtask that writes to the given writer. If total is
// 0, the task will not display a progress bar or ETA.
func (t *Task) Writer(name string, w io.Writer, total uint64, f func(*WriterTask) error) error {
	newID := uint64(time.Now().UnixNano())
	now := time.Now()
	t.ch <- &TaskEvent{
		ID:          newID,
		ParentID:    t.id,
		Name:        name,
		Total:       total,
		StartTime:   now,
		IOStartTime: now,
	}

	wt := &WriterTask{IOTask{Task{newID, t.ch}}, 0, w}

	err := f(wt)

	t.ch <- &TaskEvent{
		ID:      newID,
		EndTime: time.Now(),
		Current: wt.written,
		IsDone:  true,
		HasErr:  err != nil,
		Err:     err,
	}

	return err
}

// CopyTask tracks the progress of copying from an io.Reader to an io.Writer
type CopyTask struct {
	IOTask
	written uint64
}

// Copy copies from src to dest and updates the progress.
func (t *CopyTask) Copy(dest io.Writer, src io.Reader) (int64, error) {
	r := &countReader{
		notify: func(i int64) {
			t.ch <- &TaskEvent{
				ID:      t.id,
				Current: uint64(i),
			}
		},
		r: src,
	}

	return io.Copy(dest, r)
}

// Reset resets the progress of the task, this is useful if you want to reuse
// the same task for multiple copies.
func (t *CopyTask) Reset(total uint64) {
	t.ch <- &TaskEvent{
		ID:          t.id,
		Total:       total,
		Current:     0,
		IOStartTime: time.Now(),
	}
}

// Copier launches a new subtask that can be used to copy from an io.Reader to
// an io.Writer. If total is 0, the task will not display a progress bar or ETA.
func (t *Task) Copier(name string, total uint64, f func(*CopyTask) error) error {
	newID := uint64(time.Now().UnixNano())
	now := time.Now()
	t.ch <- &TaskEvent{
		ID:          newID,
		ParentID:    t.id,
		Name:        name,
		Total:       total,
		StartTime:   now,
		IOStartTime: now,
	}

	ct := &CopyTask{IOTask{Task{newID, t.ch}}, 0}

	err := f(ct)

	t.ch <- &TaskEvent{
		ID:      newID,
		EndTime: time.Now(),
		Current: ct.written,
		IsDone:  true,
		HasErr:  err != nil,
		Err:     err,
	}

	return err
}
