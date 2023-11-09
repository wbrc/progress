package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/wbrc/progress"
	"golang.org/x/time/rate"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigInt := make(chan os.Signal, 1)
	signal.Notify(sigInt, os.Interrupt)
	go func() {
		<-sigInt
		cancel()
	}()

	rt, done, err := progress.DisplayProgress(os.Stdout, "build image", "auto")
	if err != nil {
		panic(err)
	}
	defer func() { <-done }()
	defer rt.Close()

	err = rt.Execute("get image", LoadData(ctx))
	if err != nil {
		return
	}

	err = rt.Execute("build image", func(t progress.Task) error {
		return chill(ctx, 10*time.Second)
	})
	if err != nil {
		return
	}
}

func LoadData(ctx context.Context) func(t progress.Task) error {
	return func(t progress.Task) error {
		err := t.Copier("load image", 0, func(ct progress.CopyTask) (loadError error) {

			subtask1 := make(chan error)
			go func() {
				subtask1 <- ct.Execute("some subtask", func(t progress.Task) error {
					return chill(ctx, 2*time.Second)
				})
			}()
			defer func() {
				loadError = errors.Join(loadError, <-subtask1)
			}()

			subtask2 := make(chan error)
			go func() {
				subtask2 <- ct.Execute("some subtask", func(t progress.Task) error {
					return chill(ctx, 5*time.Second)
				})
			}()
			defer func() {
				loadError = errors.Join(loadError, <-subtask2)
			}()

			// ct.DisplayRate(true)
			// ct.DisplayETA(true)
			ct.DisplayBar(true)
			ct.Reset(32 * 1024 * 1024)
			ct.Name("download image")

			_, err := ct.Copy(io.Discard, rateReader(io.LimitReader(rand.Reader, 32*1024*1024), 4*1024*1024))
			if err != nil {
				return fmt.Errorf("failed to download: %w", err)
			}

			ct.Reset(64 * 1024 * 1024)
			ct.Name("extract image")

			_, err = ct.Copy(io.Discard, rateReader(io.LimitReader(rand.Reader, 64*1024*1024), 5*1024*1024))
			if err != nil {
				return fmt.Errorf("failed to extract: %w", err)
			}

			return nil
		})
		if err != nil {
			return err
		}

		return nil
	}
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

func chill(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		if !t.Stop() {
			<-t.C
		}
		return ctx.Err()
	}
}
