package progress

import (
	"io"
	"time"
)

type taskLogger struct {
	ch chan *TaskEvent
	id uint64
}

// Write writes logs to the task.
func (l *taskLogger) Write(p []byte) (int, error) {
	pp := make([]byte, len(p))
	copy(pp, p)
	l.ch <- &TaskEvent{
		ID:   l.id,
		Logs: pp,
	}
	return len(p), nil
}

// Task is the interface that represents a task execution. It can be used to
// execute subtasks and write logs.
type Task interface {
	// Logger returns a writer that can be used to write logs to the task.
	Logger() io.Writer

	// Name updates the name (title) of the task.
	Name(name string)

	// Execute executes a subtask in f. If f returns an error the task will be
	// marked as failed.
	Execute(name string, f func(Task) error) error

	// Reader executes a subtask that reads from r in f. If total is unknown
	// leave it as 0 and only the current progress will be displayed. If f
	// returns an error the task will be marked as failed.
	Reader(name string, r io.Reader, total uint64, f func(ReaderTask) error) error

	// Writer executes a subtask that writes to w in f. If total is unknown
	// leave it as 0 and only the current progress will be displayed. If f
	// returns an error the task will be marked as failed.
	Writer(name string, w io.Writer, total uint64, f func(WriterTask) error) error

	// Copier executes a subtask that provides a Copy method to copy from src
	// to dest. If total is unknown leave it as 0 and only the current progress
	// will be displayed. If f returns an error the task will be marked as
	// failed.
	Copier(name string, total uint64, f func(CopyTask) error) error

	// Cached will mark the task as cached. Cached tasks will be displayed
	// differently when they are done.
	Cached()
}

// ReaderTask is a Task that can be used to read from a reader and update the
// progress.
type ReaderTask interface {
	Task
	io.Reader
	DisplayRate(bool)
	DisplayETA(bool)
}

// WriterTask is a Task that can be used to write to a writer and update the
// progress.
type WriterTask interface {
	Task
	io.Writer
	DisplayRate(bool)
	DisplayETA(bool)
}

// CopyTask is a Task that can be used to copy from a reader to a writer and
// update the progress.
type CopyTask interface {
	Task
	Copy(dest io.Writer, src io.Reader) (int64, error)
	Reset(total uint64)
	DisplayRate(bool)
	DisplayETA(bool)
}

var _ Task = &taskExecutor{}

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

	EnableDisplayETA  bool // true if displaying the ETA should be enabled, only used for io tasks
	DisableDisplayETA bool // true if displaying the ETA should be disabled, only used for io tasks

	HasErr bool  // true if the task has an error
	Err    error // error of the task, will be displayed in the task body when all tasks are done

	Logs []byte // logs of the task, will be displayed in the task body
}

type taskExecutor struct {
	id  uint64
	ch  chan *TaskEvent
	log *taskLogger
}

func (t *taskExecutor) Logger() io.Writer {
	return t.log
}

func (t *taskExecutor) Name(name string) {
	t.ch <- &TaskEvent{
		ID:   t.id,
		Name: name,
	}
}

func (t *taskExecutor) Execute(name string, f func(Task) error) error {
	newID := uint64(time.Now().UnixNano())
	now := time.Now()
	t.ch <- &TaskEvent{
		ID:          newID,
		ParentID:    t.id,
		Name:        name,
		StartTime:   now,
		IOStartTime: now,
	}

	err := f(&taskExecutor{newID, t.ch, &taskLogger{t.ch, newID}})

	t.ch <- &TaskEvent{
		ID:      newID,
		EndTime: time.Now(),
		IsDone:  true,
		HasErr:  err != nil,
		Err:     err,
	}

	return err
}

type readerTask struct {
	taskExecutor
	read uint64
	r    io.Reader
}

// Read reads from the underlying reader and updates the progress.
func (t *readerTask) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)

	t.read += uint64(n)
	t.ch <- &TaskEvent{
		ID:      t.id,
		Current: t.read,
	}

	return n, err
}

func (t *readerTask) DisplayRate(b bool) {
	EnableDisplayRate := b
	DisableDisplayRate := !b
	t.ch <- &TaskEvent{
		ID:                 t.id,
		EnableDisplayRate:  EnableDisplayRate,
		DisableDisplayRate: DisableDisplayRate,
	}
}

func (t *readerTask) DisplayETA(b bool) {
	EnableDisplayETA := b
	DisableDisplayETA := !b
	t.ch <- &TaskEvent{
		ID:                t.id,
		EnableDisplayETA:  EnableDisplayETA,
		DisableDisplayETA: DisableDisplayETA,
	}
}

func (t *taskExecutor) Reader(name string, r io.Reader, total uint64, f func(ReaderTask) error) error {
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

	rt := &readerTask{taskExecutor{newID, t.ch, &taskLogger{t.ch, newID}}, 0, r}

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

type writerTask struct {
	taskExecutor
	written uint64
	w       io.Writer
}

// Write writes to the underlying writer and updates the progress.
func (t *writerTask) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)

	t.written += uint64(n)
	t.ch <- &TaskEvent{
		ID:      t.id,
		Current: t.written,
	}

	return n, err
}

func (t *writerTask) DisplayRate(b bool) {
	EnableDisplayRate := b
	DisableDisplayRate := !b
	t.ch <- &TaskEvent{
		ID:                 t.id,
		EnableDisplayRate:  EnableDisplayRate,
		DisableDisplayRate: DisableDisplayRate,
	}
}

func (t *writerTask) DisplayETA(b bool) {
	EnableDisplayETA := b
	DisableDisplayETA := !b
	t.ch <- &TaskEvent{
		ID:                t.id,
		EnableDisplayETA:  EnableDisplayETA,
		DisableDisplayETA: DisableDisplayETA,
	}
}

func (t *taskExecutor) Writer(name string, w io.Writer, total uint64, f func(WriterTask) error) error {
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

	wt := &writerTask{taskExecutor{newID, t.ch, &taskLogger{t.ch, newID}}, 0, w}

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

type copyTask struct {
	taskExecutor
	written uint64
}

// Copy copies from src to dest and updates the progress.
func (t *copyTask) Copy(dest io.Writer, src io.Reader) (int64, error) {
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

func (t *copyTask) Reset(total uint64) {
	t.ch <- &TaskEvent{
		ID:          t.id,
		Total:       total,
		Current:     0,
		IOStartTime: time.Now(),
	}
}

func (t *copyTask) DisplayRate(b bool) {
	EnableDisplayRate := b
	DisableDisplayRate := !b
	t.ch <- &TaskEvent{
		ID:                 t.id,
		EnableDisplayRate:  EnableDisplayRate,
		DisableDisplayRate: DisableDisplayRate,
	}
}

func (t *copyTask) DisplayETA(b bool) {
	EnableDisplayETA := b
	DisableDisplayETA := !b
	t.ch <- &TaskEvent{
		ID:                t.id,
		EnableDisplayETA:  EnableDisplayETA,
		DisableDisplayETA: DisableDisplayETA,
	}
}

func (t *taskExecutor) Copier(name string, total uint64, f func(CopyTask) error) error {
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

	ct := &copyTask{taskExecutor{newID, t.ch, &taskLogger{t.ch, newID}}, 0}

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
func (t *taskExecutor) Cached() {
	t.ch <- &TaskEvent{
		ID:     t.id,
		Cached: true,
	}
}
