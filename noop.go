package progress

import "github.com/containerd/console"

type noopConsole struct{}

func (noopConsole) Read([]byte) (int, error) {
	return 0, nil
}

func (noopConsole) Write([]byte) (int, error) {
	return 0, nil
}

func (noopConsole) Close() error {
	return nil
}

func (noopConsole) Fd() uintptr {
	return 0
}

func (noopConsole) Name() string {
	return ""
}

func (noopConsole) Resize(console.WinSize) error {
	return nil
}

func (noopConsole) ResizeFrom(console.Console) error {
	return nil
}

func (noopConsole) SetRaw() error {
	return nil
}

func (noopConsole) DisableEcho() error {
	return nil
}

func (noopConsole) Reset() error {
	return nil
}

func (noopConsole) Size() (console.WinSize, error) {
	return console.WinSize{}, nil
}
