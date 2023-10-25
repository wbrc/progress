package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/wbrc/progress"
	"golang.org/x/time/rate"
)

func main() {
	rt, done, err := progress.DisplayProgress(os.Stdout, "build image", "auto")
	if err != nil {
		panic(err)
	}
	defer func() { <-done }()
	defer rt.Close()

	err = rt.Execute("get image", LoadData)
	if err != nil {
		return
	}
}

func LoadData(t progress.Task) error {

	err := t.Copier("load image", 0, func(ct progress.CopyTask) error {

		ct.DisplayRate(true)
		ct.Reset(32 * 1024 * 1024)
		ct.Name("download image")

		_, err := ct.Copy(io.Discard, rateReader(io.LimitReader(rand.Reader, 32*1024*1024), 4*1024*1024))
		if err != nil {
			return fmt.Errorf("failed to download: %w", err)
		}

		ct.Reset(32 * 1024 * 1024)
		ct.Name("extract image")

		_, err = ct.Copy(io.Discard, rateReader(io.LimitReader(rand.Reader, 32*1024*1024), 5*1024*1024))
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
