package main

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"time"

	"github.com/wbrc/progress"
)

func main() {

	p, done, err := progress.DisplayProgress(os.Stdout, "build stuff", "auto")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to display progress: %v\n", err)
		os.Exit(1)
	}

	errChan := make(chan error)
	go func() {
		err := Work(p)
		if err != nil {
			errChan <- err
		}
		close(errChan)
	}()

	<-done

	if err := <-errChan; err != nil {
		fmt.Fprintf(os.Stderr, "failed to build: %v\n", err)
		os.Exit(1)
	}
}

func Work(p *progress.RootTask) (buildError error) {
	defer p.Close()

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
		for i := 0; i < 200; i++ {
			time.Sleep(time.Duration(rand.Intn(10))*time.Millisecond + 5*time.Millisecond)
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
