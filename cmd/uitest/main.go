package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"time"

	"github.com/wbrc/progress"
)

var failWithErr = false

func init() {
	flag.BoolVar(&failWithErr, "fail", false, "fail with error")
}

func main() {
	flag.Parse()

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

					return nil
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
		if buildError == nil {
			return
		}

		if err := p.Execute("cleanup", func(t *progress.Task) error {
			time.Sleep(2 * time.Second)
			return nil
		}); err != nil {
			buildError = errors.Join(buildError, fmt.Errorf("failed to cleanup: %w", err))
		}
	}()

	err = p.Execute("build image", func(t *progress.Task) error {
		for i := 0; i < 10; i++ {
			time.Sleep(time.Duration(rand.Intn(100))*time.Millisecond + 50*time.Millisecond)
			fmt.Fprintf(t.Log, "some line %d\n", i)
		}

		err := t.Execute("build subimage", func(t *progress.Task) error {
			err := t.Execute("build subsubimage", func(t *progress.Task) error {
				time.Sleep(2 * time.Second)
				if failWithErr {
					return errors.New("some err")
				}
				return nil
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

	err = p.Writer("push image", io.Discard, 0, func(t *progress.WriterTask) error {
		_, err := io.Copy(t, io.LimitReader(rand.New(rand.NewSource(0)), 100000000))
		if err != nil {
			return fmt.Errorf("failed to write: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	return nil
}
