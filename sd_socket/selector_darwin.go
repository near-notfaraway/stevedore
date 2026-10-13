//go:build darwin
// +build darwin

package sd_socket

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const kqueueWakeIdent = 1

type kqueue struct {
	fd         int
	evs        []unix.Kevent_t
	polled     []PolledEvent
	registered map[int]SelectorEvent
	et         bool
}

func NewSelector(eventSize int, et bool) (Selector, error) {
	if eventSize < 1 {
		eventSize = DefaultEventSize
	}

	fd, err := unix.Kqueue()
	if err != nil {
		return nil, os.NewSyscallError("kqueue", err)
	}
	unix.CloseOnExec(fd)

	wakeEvent := unix.Kevent_t{
		Ident:  kqueueWakeIdent,
		Filter: unix.EVFILT_USER,
		Flags:  unix.EV_ADD | unix.EV_CLEAR,
	}
	if _, err = unix.Kevent(fd, []unix.Kevent_t{wakeEvent}, nil, nil); err != nil {
		_ = unix.Close(fd)
		return nil, os.NewSyscallError("kevent add wake", err)
	}

	return &kqueue{
		fd:         fd,
		evs:        make([]unix.Kevent_t, eventSize),
		polled:     make([]PolledEvent, 0, eventSize),
		registered: make(map[int]SelectorEvent),
		et:         et,
	}, nil
}

func (s *kqueue) Close() error {
	if err := unix.Close(s.fd); err != nil {
		return os.NewSyscallError("close kqueue", err)
	}
	return nil
}

func (s *kqueue) Add(fd int, ev SelectorEvent) error {
	if _, exists := s.registered[fd]; exists {
		return os.NewSyscallError("kevent add", unix.EEXIST)
	}
	changes, err := s.addChanges(fd, ev)
	if err != nil {
		return err
	}
	if _, err = unix.Kevent(s.fd, changes, nil, nil); err != nil {
		return os.NewSyscallError("kevent add", err)
	}
	s.registered[fd] = ev
	return nil
}

func (s *kqueue) Mod(fd int, ev SelectorEvent) error {
	old, exists := s.registered[fd]
	if !exists {
		return os.NewSyscallError("kevent mod", unix.ENOENT)
	}
	if _, err := selectorFilters(ev); err != nil {
		return err
	}
	if old == ev {
		return nil
	}

	changes := make([]unix.Kevent_t, 0, 2)
	for _, filter := range []int16{unix.EVFILT_READ, unix.EVFILT_WRITE} {
		oldHas := hasSelectorFilter(old, filter)
		newHas := hasSelectorFilter(ev, filter)
		switch {
		case oldHas && !newHas:
			changes = append(changes, unix.Kevent_t{
				Ident:  uint64(fd),
				Filter: filter,
				Flags:  unix.EV_DELETE,
			})
		case !oldHas && newHas:
			changes = append(changes, s.addChange(fd, filter))
		}
	}
	if _, err := unix.Kevent(s.fd, changes, nil, nil); err != nil {
		return os.NewSyscallError("kevent mod", err)
	}
	s.registered[fd] = ev
	return nil
}

func (s *kqueue) Del(fd int) error {
	old, exists := s.registered[fd]
	if !exists {
		return os.NewSyscallError("kevent delete", unix.ENOENT)
	}

	filters, _ := selectorFilters(old)
	changes := make([]unix.Kevent_t, 0, len(filters))
	for _, filter := range filters {
		changes = append(changes, unix.Kevent_t{
			Ident:  uint64(fd),
			Filter: filter,
			Flags:  unix.EV_DELETE,
		})
	}
	if _, err := unix.Kevent(s.fd, changes, nil, nil); err != nil {
		return os.NewSyscallError("kevent delete", err)
	}
	delete(s.registered, fd)
	return nil
}

func (s *kqueue) Wake() error {
	event := unix.Kevent_t{
		Ident:  kqueueWakeIdent,
		Filter: unix.EVFILT_USER,
		Fflags: unix.NOTE_TRIGGER,
	}
	if _, err := unix.Kevent(s.fd, []unix.Kevent_t{event}, nil, nil); err != nil {
		return os.NewSyscallError("kevent trigger wake", err)
	}
	return nil
}

func (s *kqueue) Polling(ctx context.Context, timeoutMs int, handle func(evs []PolledEvent)) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		var timeout *unix.Timespec
		if timeoutMs >= 0 {
			value := unix.NsecToTimespec(int64(timeoutMs) * int64(time.Millisecond))
			timeout = &value
		}
		n, err := unix.Kevent(s.fd, nil, s.evs, timeout)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return os.NewSyscallError("kevent wait", err)
		}

		s.polled = s.polled[:0]
		for i := 0; i < n; i++ {
			event := s.evs[i]
			if event.Flags&unix.EV_ERROR != 0 && event.Data != 0 {
				return os.NewSyscallError("kevent event", unix.Errno(event.Data))
			}
			if event.Filter == unix.EVFILT_USER && event.Ident == kqueueWakeIdent {
				continue
			}

			var events SelectorEvent
			switch event.Filter {
			case unix.EVFILT_READ:
				events = SelectorEventRead
			case unix.EVFILT_WRITE:
				events = SelectorEventWrite
			default:
				continue
			}
			s.polled = append(s.polled, PolledEvent{
				FD:     int(event.Ident),
				Events: events,
			})
		}
		handle(s.polled)
	}
}

func (s *kqueue) addChanges(fd int, ev SelectorEvent) ([]unix.Kevent_t, error) {
	filters, err := selectorFilters(ev)
	if err != nil {
		return nil, err
	}
	changes := make([]unix.Kevent_t, 0, len(filters))
	for _, filter := range filters {
		changes = append(changes, s.addChange(fd, filter))
	}
	return changes, nil
}

func (s *kqueue) addChange(fd int, filter int16) unix.Kevent_t {
	flags := uint16(unix.EV_ADD | unix.EV_ENABLE)
	if s.et {
		flags |= unix.EV_CLEAR
	}
	return unix.Kevent_t{
		Ident:  uint64(fd),
		Filter: filter,
		Flags:  flags,
	}
}

func selectorFilters(ev SelectorEvent) ([]int16, error) {
	switch ev {
	case SelectorEventRead:
		return []int16{unix.EVFILT_READ}, nil
	case SelectorEventWrite:
		return []int16{unix.EVFILT_WRITE}, nil
	case SelectorEventReadWrite:
		return []int16{unix.EVFILT_READ, unix.EVFILT_WRITE}, nil
	default:
		return nil, fmt.Errorf("unknown selector event type: %d", ev)
	}
}

func hasSelectorFilter(ev SelectorEvent, filter int16) bool {
	switch filter {
	case unix.EVFILT_READ:
		return ev == SelectorEventRead || ev == SelectorEventReadWrite
	case unix.EVFILT_WRITE:
		return ev == SelectorEventWrite || ev == SelectorEventReadWrite
	default:
		return false
	}
}
