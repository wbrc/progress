package progress

import (
	"io"
	"time"
)

func New(ch chan *TaskEvent) *TaskExecutor {
	return &TaskExecutor{
		ch: ch,
	}
}

type TaskExecutor struct {
	id uint64
	ch chan *TaskEvent
}

func (t *TaskExecutor) Execute(name string, f func(*Task) error) error {
	newID := uint64(time.Now().UnixNano())
	t.ch <- &TaskEvent{
		ID:        newID,
		ParentID:  t.id,
		Name:      name,
		StartTime: time.Now(),
	}

	err := f(&Task{TaskExecutor{newID, t.ch}})

	t.ch <- &TaskEvent{
		ID:      newID,
		EndTime: time.Now(),
		IsDone:  true,
		HasErr:  err != nil,
		Err:     err,
	}

	return err
}

type ReaderTask struct {
	TaskExecutor
	read uint64
	r    io.Reader
}

func (t *ReaderTask) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)

	t.read += uint64(n)
	t.ch <- &TaskEvent{
		ID:      t.id,
		Current: t.read,
	}

	return n, err
}

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

type WriterTask struct {
	TaskExecutor
	written uint64
	w       io.Writer
}

func (t *WriterTask) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)

	t.written += uint64(n)
	t.ch <- &TaskEvent{
		ID:      t.id,
		Current: t.written,
	}

	return n, err
}

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

type Task struct {
	TaskExecutor
}

func (t *Task) Write(p []byte) (int, error) {
	pp := make([]byte, len(p))
	copy(pp, p)
	t.ch <- &TaskEvent{
		ID:   t.id,
		Logs: pp,
	}
	return len(p), nil
}
