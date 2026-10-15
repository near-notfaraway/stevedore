//go:build linux

package sd_socket

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestEpollerLinuxLifecycle(t *testing.T) {
	selector, err := NewSelector(0, true)
	require.NoError(t, err)
	epoller := selector.(*epoller)
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
	level := &epoller{}
	read, err := level.epollEvents(SelectorEventRead)
	require.NoError(t, err)
	require.Equal(t, uint32(unix.EPOLLIN), read)

	write, err := level.epollEvents(SelectorEventWrite)
	require.NoError(t, err)
	require.Equal(t, uint32(unix.EPOLLOUT), write)

	readWrite, err := level.epollEvents(SelectorEventReadWrite)
	require.NoError(t, err)
	require.Equal(t, uint32(unix.EPOLLIN|unix.EPOLLOUT), readWrite)

	edge := &epoller{et: true}
	events, err := edge.epollEvents(SelectorEventRead)
	require.NoError(t, err)
	require.NotZero(t, events&unix.EPOLLET)

	_, err = level.epollEvents(SelectorEvent(255))
	require.Error(t, err)
}

func TestEventFDLinux(t *testing.T) {
	fd, err := newEventFD()
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(fd) })

	statusFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	require.NoError(t, err)
	require.NotZero(t, statusFlags&unix.O_NONBLOCK)
	descriptorFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	require.NoError(t, err)
	require.NotZero(t, descriptorFlags&unix.FD_CLOEXEC)

	require.NoError(t, notifyEventFD(fd))
	require.NoError(t, notifyEventFD(fd))
	require.NoError(t, drainEventFD(fd))
	require.NoError(t, drainEventFD(fd))

	require.Error(t, notifyEventFD(-1))
	require.Error(t, drainEventFD(-1))
}

func TestSelectorPollingReadAndWakeLinux(t *testing.T) {
	selector, err := newEpoller(8, true)
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
	selector, err := newEpoller(1, false)
	require.NoError(t, err)
	defer selector.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, selector.Polling(ctx, -1, func([]PolledEvent) {
		t.Fatal("handler must not run for an already canceled context")
	}))
}

func TestEpollerLinuxReportsClosedDescriptorErrors(t *testing.T) {
	selector, err := newEpoller(1, false)
	require.NoError(t, err)
	require.NoError(t, selector.Close())

	require.Error(t, selector.Wake())
	require.Error(t, selector.Add(0, SelectorEventRead))
	require.Error(t, selector.Mod(0, SelectorEventRead))
	require.Error(t, selector.Del(0))
	require.Error(t, selector.Close())
}

func TestNewSelectorConvey(t *testing.T) {
	convey.Convey("Test NewSelector", t, func() {
		convey.Convey("creates an epoller", func() {
			selector, err := NewSelector(1, false)
			convey.So(err, convey.ShouldBeNil)
			convey.So(selector, convey.ShouldHaveSameTypeAs, &epoller{})
			convey.So(selector.Close(), convey.ShouldBeNil)
		})
	})
}

func TestNewEpollerConvey(t *testing.T) {
	convey.Convey("Test newEpoller", t, func() {
		convey.Convey("initializes epoll and wake descriptors", func() {
			selector, err := newEpoller(2, true)
			convey.So(err, convey.ShouldBeNil)
			convey.So(selector.(*epoller).evs, convey.ShouldHaveLength, 2)
			convey.So(selector.Close(), convey.ShouldBeNil)
		})
	})
}

func TestEpollerCloseConvey(t *testing.T) {
	convey.Convey("Test epoller.Close", t, func() {
		convey.Convey("closes owned descriptors", func() {
			selector, _ := newEpoller(1, false)
			convey.So(selector.Close(), convey.ShouldBeNil)
			convey.So(selector.Close(), convey.ShouldNotBeNil)
		})
	})
}

func TestEpollerAddConvey(t *testing.T) {
	convey.Convey("Test epoller.Add", t, func() {
		convey.Convey("rejects an invalid event", func() {
			selector, _ := newEpoller(1, false)
			defer selector.Close()
			convey.So(selector.Add(0, SelectorEvent(255)), convey.ShouldNotBeNil)
		})
	})
}

func TestEpollerModConvey(t *testing.T) {
	convey.Convey("Test epoller.Mod", t, func() {
		convey.Convey("rejects an invalid event", func() {
			selector, _ := newEpoller(1, false)
			defer selector.Close()
			convey.So(selector.Mod(0, SelectorEvent(255)), convey.ShouldNotBeNil)
		})
	})
}

func TestEpollerControlConvey(t *testing.T) {
	convey.Convey("Test epoller.control", t, func() {
		convey.Convey("rejects an invalid event before syscall", func() {
			convey.So((&epoller{}).control(unix.EPOLL_CTL_ADD, 0, SelectorEvent(255)), convey.ShouldNotBeNil)
		})
	})
}

func TestEpollerEpollEventsConvey(t *testing.T) {
	convey.Convey("Test epoller.epollEvents", t, func() {
		convey.Convey("maps read and write", func() {
			events, err := (&epoller{et: true}).epollEvents(SelectorEventReadWrite)
			convey.So(err, convey.ShouldBeNil)
			convey.So(events&unix.EPOLLIN, convey.ShouldNotEqual, 0)
			convey.So(events&unix.EPOLLOUT, convey.ShouldNotEqual, 0)
			convey.So(events&unix.EPOLLET, convey.ShouldNotEqual, 0)
		})
	})
}

func TestEpollerDelConvey(t *testing.T) {
	convey.Convey("Test epoller.Del", t, func() {
		convey.Convey("reports an unknown descriptor", func() {
			selector, _ := newEpoller(1, false)
			defer selector.Close()
			convey.So(selector.Del(-1), convey.ShouldNotBeNil)
		})
	})
}

func TestEpollerWakeConvey(t *testing.T) {
	convey.Convey("Test epoller.Wake", t, func() {
		convey.Convey("notifies the selector", func() {
			selector, _ := newEpoller(1, false)
			defer selector.Close()
			convey.So(selector.Wake(), convey.ShouldBeNil)
		})
	})
}

func TestEpollerPollingConvey(t *testing.T) {
	convey.Convey("Test epoller.Polling", t, func() {
		convey.Convey("returns for canceled context", func() {
			selector, _ := newEpoller(1, false)
			defer selector.Close()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			convey.So(selector.Polling(ctx, -1, func([]PolledEvent) {}), convey.ShouldBeNil)
		})
	})
}

func TestNewEventFDConvey(t *testing.T) {
	convey.Convey("Test newEventFD", t, func() {
		convey.Convey("creates an event descriptor", func() {
			fd, err := newEventFD()
			convey.So(err, convey.ShouldBeNil)
			convey.So(fd, convey.ShouldBeGreaterThanOrEqualTo, 0)
			convey.So(unix.Close(fd), convey.ShouldBeNil)
		})
	})
}

func TestNotifyEventFDConvey(t *testing.T) {
	convey.Convey("Test notifyEventFD", t, func() {
		convey.Convey("writes a notification", func() {
			fd, _ := newEventFD()
			defer unix.Close(fd)
			convey.So(notifyEventFD(fd), convey.ShouldBeNil)
		})
	})
}

func TestDrainEventFDConvey(t *testing.T) {
	convey.Convey("Test drainEventFD", t, func() {
		convey.Convey("consumes notifications", func() {
			fd, _ := newEventFD()
			defer unix.Close(fd)
			convey.So(notifyEventFD(fd), convey.ShouldBeNil)
			convey.So(drainEventFD(fd), convey.ShouldBeNil)
		})
	})
}
