package progress

import (
	"fmt"
	"io"
	"time"

	"github.com/containerd/console"
	"golang.org/x/time/rate"
)

type progressRenderer interface {
	update(te *TaskEvent)
	render(w io.Writer, width int, showError bool)
}

// Processes events from a channel and renders them to the console or trace. The
// mode can be "auto", "tty" or "plain". In "auto" mode, the console is used if
// available. In "tty" mode, the console is used and an error is returned if it
// is not available. In "plain" mode, the trace is used.
// When the events channel is closed, the last state is rendered and the
// function returns. The returned channel is closed when the rendering is
// complete.
// You'll most likely want to use [DisplayProgress] instead of this function.
func ProcessEvents(f console.File, name, mode string, events <-chan *TaskEvent) (<-chan struct{}, error) {

	var renderer progressRenderer = newTraceRenderer(name)
	var cons console.Console = noopConsole{}

	switch mode {
	case "auto", "tty":
		if c, err := console.ConsoleFromFile(f); err == nil {
			cons = c
			renderer = newConsoleRenderer(name)
		} else if mode == "tty" {
			return nil, fmt.Errorf("failed to open console: %s", err)
		}

	case "plain":
	default:
		return nil, fmt.Errorf("unknown mode %q", mode)
	}

	tickRate := 150 * time.Millisecond
	rateLimit := 100 * time.Millisecond

	t := time.NewTicker(tickRate)
	r := rate.NewLimiter(rate.Every(rateLimit), 1)
	doneChan := make(chan struct{})

	go func() {
		for done := false; !done; {
			select {
			case <-t.C:
			case e, ok := <-events:
				if !ok {
					done = true
				} else {
					renderer.update(e)
				}
			}

			if done || r.Allow() {
				size, err := cons.Size()
				if err != nil {
					size = console.WinSize{Width: 80}
				}

				renderer.render(f, int(size.Width), done)
				t.Stop()
				t = time.NewTicker(tickRate)
			}
		}
		close(doneChan)
	}()

	return doneChan, nil
}
