package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"time"

	"github.com/wbrc/progress"
	"golang.org/x/time/rate"
)

var (
	failWithErr   = false
	failDownload2 = false
)

func init() {
	flag.BoolVar(&failWithErr, "fail", false, "fail with error")
	flag.BoolVar(&failDownload2, "faildl", false, "fail download step with error")
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
				size := uint64(rand.Intn(10000000) + 10000000)
				errs <- t.Reader(fmt.Sprintf("fetching %d", idx), rand.New(rand.NewSource(0)), size, func(rt *progress.ReaderTask) error {

					if failDownload2 && idx == 1 {
						size /= 2
					}

					lr := rateReader(io.LimitReader(rt, int64(size)), rand.Intn(5300121)+5300121)
					_, err := io.Copy(io.Discard, lr)
					if err != nil {
						return fmt.Errorf("failed to read: %w", err)
					}

					if failDownload2 && idx == 1 {
						return errors.New("download err")
					}

					return nil
				})
			}(i)
		}

		var allErrs error
		for i := 0; i < 3; i++ {
			allErrs = errors.Join(allErrs, <-errs)
		}

		if allErrs != nil {
			return fmt.Errorf("failed to fetch image: %w", allErrs)
		}

		return nil
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
			time.Sleep(time.Duration(rand.Intn(1000))*time.Millisecond + 500*time.Millisecond)
			err := t.Execute("build subsubimage", func(t *progress.Task) error {
				time.Sleep(time.Duration(rand.Intn(1000))*time.Millisecond + 500*time.Millisecond)
				if failWithErr {
					return errors.New("some err")
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("failed to build subsubimage: %w", err)
			}
			time.Sleep(time.Duration(rand.Intn(1000))*time.Millisecond + 500*time.Millisecond)

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
		size := int64(rand.Intn(10000000) + 10000000)
		rr := rateReader(io.LimitReader(rand.New(rand.NewSource(0)), size), 2e7)
		_, err := io.Copy(t, rr)
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

func rateReader(r io.Reader, maxRate int) io.Reader {
	rr := &rateR{
		r: r,
		l: rate.NewLimiter(rate.Limit(maxRate), 1000*1000*1000),
	}
	rr.l.AllowN(time.Now(), 1000*1000*1000)
	return rr
}

type rateR struct {
	r io.Reader
	l *rate.Limiter
}

func (l *rateR) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	if err != nil {
		return n, err
	}

	if err := l.l.WaitN(context.Background(), n); err != nil {
		return n, err
	}

	return n, nil
}
