package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/wbrc/progress"
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
	// load big file
	resp, err := http.Get("https://fsn1-speed.hetzner.com/100MB.bin")
	if err != nil {
		return fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	err = t.Reader("download", resp.Body, uint64(resp.ContentLength), func(rt progress.ReaderTask) error {
		_, err := io.Copy(io.Discard, rt)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	err = t.Writer("extract", io.Discard, 0, func(wt progress.WriterTask) error {
		buf := make([]byte, 1024)
		for i := 0; i < 100; i++ {
			time.Sleep(100 * time.Millisecond)
			_, _ = rand.Read(buf)
			_, err := wt.Write(buf)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	return nil
}
