//go:build !windows
// +build !windows

package filewatcher

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
	"gotest.tools/gotestsum/log"
)

type terminal struct {
	ch    chan Event
	reset func()
}

func newTerminal() *terminal {
	h := &terminal{ch: make(chan Event)}
	h.Start()
	return h
}

// Start the terminal is non-blocking read mode. The terminal can be reset to
// normal mode by calling Reset.
func (r *terminal) Start() {
	if r == nil {
		return
	}
	fd := int(os.Stdin.Fd())
	reset, err := enableNonBlockingRead(fd)
	if err != nil {
		log.Warnf("failed to put terminal (fd %d) into raw mode: %v", fd, err)
		return
	}
	r.reset = reset
}

func enableNonBlockingRead(fd int) (func(), error) {
	term, err := unix.IoctlGetTermios(fd, tcGet)
	if err != nil {
		return nil, err
	}

	state := *term
	reset := func() {
		if err := unix.IoctlSetTermios(fd, tcSet, &state); err != nil {
			log.Debugf("failed to reset fd %d: %v", fd, err)
		}
	}

	term.Lflag &^= unix.ECHO | unix.ICANON
	term.Cc[unix.VMIN] = 1
	term.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, tcSet, term); err != nil {
		reset()
		return nil, err
	}
	return reset, nil
}

// Monitor the terminal for key presses. If the key press is associated with an
// action, an event will be sent to channel returned by Events.
func (r *terminal) Monitor(ctx context.Context) {
	if r == nil {
		return
	}
	in := bufio.NewReader(os.Stdin)
	for {
		char, err := in.ReadByte()
		if err != nil {
			log.Warnf("failed to read input: %v", err)
			return
		}
		log.Debugf("received byte %v (%v)", char, string(char))

		var chResume chan struct{}
		switch char {
		case 'r':
			chResume = make(chan struct{})
			r.ch <- Event{resume: chResume}
		case 'd':
			chResume = make(chan struct{})
			r.ch <- Event{Debug: true, resume: chResume}
		case 'a':
			chResume = make(chan struct{})
			r.ch <- Event{resume: chResume, PkgPath: "./..."}
		case 'l':
			chResume = make(chan struct{})
			r.ch <- Event{resume: chResume, reloadPaths: true}
		case '\n':
			fmt.Println()
			continue
		default:
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-chResume:
		}
	}
}

// Events returns a channel which will receive events when keys are pressed.
// When an event is received, the caller must close the resume channel to
// resume monitoring for events.
func (r *terminal) Events() <-chan Event {
	if r == nil {
		return nil
	}
	return r.ch
}

func (r *terminal) Reset() {
	if r != nil && r.reset != nil {
		r.reset()
	}
}
