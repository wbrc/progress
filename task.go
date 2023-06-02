package progress

import (
	"io"
	"time"
)

// TaskEvent carries all the information about tasks
type TaskEvent struct {
	ID       uint64 // unique ID for the task, must be > 0
	ParentID uint64 // ID of the parent task, 0 if no parent

	Name string // name of the task, this will be displayed in the header line

	StartTime, EndTime time.Time // start and end time of the task, used to calculate the duration
	IsDone             bool      // true if the task is done, finished tasks will be displayed differently

	// Current and Total are used to display a copy progress if Total is
	// unknown leave it as 0 and only Current will be displayed
	Current, Total uint64

	HasErr bool  // true if the task has an error
	Err    error // error of the task, will be displayed in the task body when all tasks are done

	Logs []byte // logs of the task, will be displayed in the task body
}

// TaskExecutor is a task that can be used to execute subtasks
type TaskExecutor struct {
	id uint64
	ch chan *TaskEvent
}

// Execute executes a subtask in f. If f returns an error the task will be
// marked as failed.
func (t *TaskExecutor) Execute(name string, f func(*Task) error) error {
	newID := uint64(time.Now().UnixNano())
	t.ch <- &TaskEvent{
		ID:        newID,
		ParentID:  t.id,
		Name:      name,
		StartTime: time.Now(),
	}

	err := f(&Task{TaskExecutor{newID, t.ch}, &taskLogger{t.ch, newID}})

	t.ch <- &TaskEvent{
		ID:      newID,
		EndTime: time.Now(),
		IsDone:  true,
		HasErr:  err != nil,
		Err:     err,
	}

	return err
}

// ReaderTask is a task that can be used to track progress of reading from a
// reader.
type ReaderTask struct {
	TaskExecutor
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

// Reader executes a Reader subtask in f.
func (t *TaskExecutor) Reader(name string, r io.Reader, total uint64, f func(*ReaderTask) error) error {
	newID := uint64(time.Now().UnixNano())
	t.ch <- &TaskEvent{
		ID:        newID,
		ParentID:  t.id,
		Name:      name,
		Total:     total,
		StartTime: time.Now(),
	}

	rt := &ReaderTask{TaskExecutor{newID, t.ch}, 0, r}

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

// WriterTask is a task that can be used to track progress of writing to a
// writer.
type WriterTask struct {
	TaskExecutor
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

// Writer executes a Writer subtask in f.
func (t *TaskExecutor) Writer(name string, w io.Writer, total uint64, f func(*WriterTask) error) error {
	newID := uint64(time.Now().UnixNano())
	t.ch <- &TaskEvent{
		ID:        newID,
		ParentID:  t.id,
		Name:      name,
		Total:     total,
		StartTime: time.Now(),
	}

	wt := &WriterTask{TaskExecutor{newID, t.ch}, 0, w}

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

// Task is a task or subtask that can be used to write logs.
type Task struct {
	TaskExecutor
	Log *taskLogger // Log implements io.Writer and can be used to write logs
}

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
