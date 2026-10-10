//go:build darwin
// +build darwin

package sd_socket

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSelectorDarwinLifecycle(t *testing.T) {
	selector, err := NewSelector(0, false)
	require.NoError(t, err)
	kqueue := selector.(*Kqueue)
	require.Len(t, kqueue.evs, DefaultEventSize)
	require.Len(t, kqueue.polled, 0)
	t.Cleanup(func() { _ = selector.Close() })

	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(pair[0]) })
	t.Cleanup(func() { _ = unix.Close(pair[1]) })

	require.Error(t, selector.Add(pair[0], SelectorEvent(255)))
	require.Empty(t, kqueue.registered)

	require.NoError(t, selector.Add(pair[0], SelectorEventRead))
	require.Equal(t, SelectorEventRead, kqueue.registered[pair[0]])
	require.Error(t, selector.Add(pair[0], SelectorEventRead))

	require.Error(t, selector.Mod(pair[1], SelectorEventRead))
	require.Error(t, selector.Mod(pair[0], SelectorEvent(255)))
	require.Equal(t, SelectorEventRead, kqueue.registered[pair[0]])
	require.NoError(t, selector.Mod(pair[0], SelectorEventRead))
	require.NoError(t, selector.Mod(pair[0], SelectorEventReadWrite))
	require.Equal(t, SelectorEventReadWrite, kqueue.registered[pair[0]])
	require.NoError(t, selector.Mod(pair[0], SelectorEventWrite))
	require.Equal(t, SelectorEventWrite, kqueue.registered[pair[0]])

	require.NoError(t, selector.Del(pair[0]))
	require.NotContains(t, kqueue.registered, pair[0])
	require.Error(t, selector.Del(pair[0]))
}

func TestSelectorHelpersDarwin(t *testing.T) {
	tests := []struct {
		event SelectorEvent
		want  []int16
	}{
		{event: SelectorEventRead, want: []int16{unix.EVFILT_READ}},
		{event: SelectorEventWrite, want: []int16{unix.EVFILT_WRITE}},
		{event: SelectorEventReadWrite, want: []int16{unix.EVFILT_READ, unix.EVFILT_WRITE}},
	}
	for _, tt := range tests {
		filters, err := selectorFilters(tt.event)
		require.NoError(t, err)
		require.Equal(t, tt.want, filters)
	}
	_, err := selectorFilters(SelectorEvent(255))
	require.Error(t, err)

	require.True(t, hasSelectorFilter(SelectorEventReadWrite, unix.EVFILT_READ))
	require.True(t, hasSelectorFilter(SelectorEventReadWrite, unix.EVFILT_WRITE))
	require.False(t, hasSelectorFilter(SelectorEventRead, unix.EVFILT_WRITE))
	require.False(t, hasSelectorFilter(SelectorEventWrite, unix.EVFILT_READ))
	require.False(t, hasSelectorFilter(SelectorEventReadWrite, 0))

	levelTriggered := &Kqueue{}
	require.Equal(t, uint16(unix.EV_ADD|unix.EV_ENABLE), levelTriggered.addChange(7, unix.EVFILT_READ).Flags)
	edgeTriggered := &Kqueue{et: true}
	require.NotZero(t, edgeTriggered.addChange(7, unix.EVFILT_READ).Flags&unix.EV_CLEAR)
}

func TestSelectorPollingReturnsWhenContextCanceledDarwin(t *testing.T) {
	selector, err := NewSelector(1, false)
	require.NoError(t, err)
	defer selector.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, selector.Polling(ctx, -1, func([]PolledEvent) {
		t.Fatal("handler must not run for an already canceled context")
	}))
}

func TestSelectorPollingReadAndWake(t *testing.T) {
	selector, err := NewSelector(8, true)
	require.NoError(t, err)
	defer selector.Close()

	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
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
			copied := append([]PolledEvent(nil), events...)
			eventCh <- copied
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
		t.Fatal("timed out waiting for read event")
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
		t.Fatal("timed out waiting for write event")
	}

	require.NoError(t, selector.Wake())

	select {
	case events := <-eventCh:
		require.Empty(t, events)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for selector wake")
	}

	cancel()
	require.NoError(t, selector.Wake())
	select {
	case err = <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for selector shutdown")
	}
}

func TestSelectorDarwinReportsClosedDescriptorErrors(t *testing.T) {
	selector, err := NewSelector(1, false)
	require.NoError(t, err)
	require.NoError(t, selector.Close())

	require.Error(t, selector.Wake())
	require.Error(t, selector.Add(0, SelectorEventRead))
	require.Error(t, selector.Mod(0, SelectorEventRead))
	require.Error(t, selector.Del(0))
	require.Error(t, selector.Close())
}
