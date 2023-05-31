package main

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"time"

	"github.com/wbrc/progress"

	"github.com/containerd/console"
)

func main() {

	con, err := console.ConsoleFromFile(os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open console: %v\n", err)
		os.Exit(1)
	}

	ch := make(chan *progress.TaskEvent)

	errChan := make(chan error)
	go func() {
		err := Work(ch)
		if err != nil {
			errChan <- err
		}
		close(errChan)
	}()

	progress.MainLoop(con, "build stuff", ch)

	if err := <-errChan; err != nil {
		fmt.Fprintf(os.Stderr, "failed to build: %v\n", err)
		os.Exit(1)
	}
}

func Work(ch chan *progress.TaskEvent) (buildError error) {
	defer close(ch)

	p := progress.New(ch)

	err := p.Execute("fetch image", func(t *progress.Task) error {
		errs := make(chan error)

		for i := 0; i < 3; i++ {
			go func(idx int) {
				errs <- t.Reader(fmt.Sprintf("fetching %d", idx), rand.New(rand.NewSource(0)), 100000000, func(rt *progress.ReaderTask) error {
					_, err := io.Copy(io.Discard, io.LimitReader(rt, 100000000))
					if err != nil {
						return fmt.Errorf("failed to read: %w", err)
					}

					if rand.New(rand.NewSource(time.Now().UnixNano())).Intn(4) != 0 {
						return nil
					} else {
						return errors.New("some error")
					}
				})
			}(i)
		}

		var allErrs error
		for i := 0; i < 3; i++ {
			allErrs = errors.Join(allErrs, <-errs)
		}

		return allErrs
	})
	if err != nil {
		return fmt.Errorf("failed to fetch image: %w", err)
	}

	defer func() {
		if err := p.Execute("cleanup", func(t *progress.Task) error {
			time.Sleep(2 * time.Second)
			return nil
		}); err != nil {
			buildError = errors.Join(buildError, fmt.Errorf("failed to cleanup: %w", err))
		}
	}()

	err = p.Execute("build image", func(t *progress.Task) error {
		for i := 0; i < 20; i++ {
			time.Sleep(time.Duration(rand.Intn(50))*time.Millisecond + 50*time.Millisecond)
			fmt.Fprintf(t, "some line %d\n", i)
		}

		err := t.Execute("build subimage", func(t *progress.Task) error {
			err := t.Execute("build subsubimage", func(t *progress.Task) error {
				time.Sleep(2 * time.Second)
				return errors.New("some err")
			})
			if err != nil {
				return fmt.Errorf("failed to build subsubimage: %w", err)
			}

			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to build subimage: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}

	return nil
}
