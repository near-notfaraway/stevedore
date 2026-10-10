//go:build linux

package sd_socket

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestEpollerLinuxLifecycle(t *testing.T) {
	selector, err := NewSelector(0, true)
	require.NoError(t, err)
	epoller := selector.(*Epoller)
	require.Len(t, epoller.evs, DefaultEventSize)
	require.Len(t, epoller.polled, 0)
	t.Cleanup(func() { _ = selector.Close() })

	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(pair[0]) })
	t.Cleanup(func() { _ = unix.Close(pair[1]) })

	require.Error(t, selector.Add(pair[0], SelectorEvent(255)))
	require.NoError(t, selector.Add(pair[0], SelectorEventRead))
	require.Error(t, selector.Add(pair[0], SelectorEventRead))
	require.NoError(t, selector.Mod(pair[0], SelectorEventReadWrite))
	require.NoError(t, selector.Mod(pair[0], SelectorEventWrite))
	require.NoError(t, selector.Del(pair[0]))
	require.Error(t, selector.Del(pair[0]))
	require.Error(t, selector.Mod(pair[0], SelectorEventRead))
}

func TestEpollEventsLinux(t *testing.T) {
	level := &Epoller{}
	read, err := level.epollEvents(SelectorEventRead)
	require.NoError(t, err)
	require.Equal(t, uint32(unix.EPOLLIN), read)

	write, err := level.epollEvents(SelectorEventWrite)
	require.NoError(t, err)
	require.Equal(t, uint32(unix.EPOLLOUT), write)

	readWrite, err := level.epollEvents(SelectorEventReadWrite)
	require.NoError(t, err)
	require.Equal(t, uint32(unix.EPOLLIN|unix.EPOLLOUT), readWrite)

	edge := &Epoller{et: true}
	events, err := edge.epollEvents(SelectorEventRead)
	require.NoError(t, err)
	require.NotZero(t, events&unix.EPOLLET)

	_, err = level.epollEvents(SelectorEvent(255))
	require.Error(t, err)
}

func TestEventFDLinux(t *testing.T) {
	fd, err := NewEventFD()
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(fd) })

	statusFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	require.NoError(t, err)
	require.NotZero(t, statusFlags&unix.O_NONBLOCK)
	descriptorFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	require.NoError(t, err)
	require.NotZero(t, descriptorFlags&unix.FD_CLOEXEC)

	require.NoError(t, NotifyEventFD(fd))
	require.NoError(t, NotifyEventFD(fd))
	require.NoError(t, DrainEventFD(fd))
	require.NoError(t, DrainEventFD(fd))

	require.Error(t, NotifyEventFD(-1))
	require.Error(t, DrainEventFD(-1))
}

func TestSelectorPollingReadAndWakeLinux(t *testing.T) {
	selector, err := NewEpoller(8, true)
	require.NoError(t, err)
	defer selector.Close()

	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	require.NoError(t, err)
	defer unix.Close(pair[0])
	defer unix.Close(pair[1])
	require.NoError(t, selector.Add(pair[0], SelectorEventRead))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh := make(chan []PolledEvent, 2)
	errCh := make(chan error, 1)
	go func() {
		errCh <- selector.Polling(ctx, 1000, func(events []PolledEvent) {
			eventCh <- append([]PolledEvent(nil), events...)
		})
	}()

	_, err = unix.Write(pair[1], []byte{1})
	require.NoError(t, err)
	select {
	case events := <-eventCh:
		require.Equal(t, []PolledEvent{{
			FD:     pair[0],
			Events: SelectorEventRead,
		}}, events)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for epoll read event")
	}

	var buf [1]byte
	_, err = unix.Read(pair[0], buf[:])
	require.NoError(t, err)

	require.NoError(t, selector.Mod(pair[0], SelectorEventWrite))
	select {
	case events := <-eventCh:
		require.Equal(t, []PolledEvent{{
			FD:     pair[0],
			Events: SelectorEventWrite,
		}}, events)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for epoll write event")
	}

	require.NoError(t, selector.Wake())
	select {
	case events := <-eventCh:
		require.Empty(t, events)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for epoll wake")
	}

	cancel()
	require.NoError(t, selector.Wake())
	select {
	case err = <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for epoll shutdown")
	}
}

func TestSelectorPollingReturnsWhenContextCanceledLinux(t *testing.T) {
	selector, err := NewEpoller(1, false)
	require.NoError(t, err)
	defer selector.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, selector.Polling(ctx, -1, func([]PolledEvent) {
		t.Fatal("handler must not run for an already canceled context")
	}))
}

func TestEpollerLinuxReportsClosedDescriptorErrors(t *testing.T) {
	selector, err := NewEpoller(1, false)
	require.NoError(t, err)
	require.NoError(t, selector.Close())

	require.Error(t, selector.Wake())
	require.Error(t, selector.Add(0, SelectorEventRead))
	require.Error(t, selector.Mod(0, SelectorEventRead))
	require.Error(t, selector.Del(0))
	require.Error(t, selector.Close())
}
