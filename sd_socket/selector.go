package sd_socket

import "context"

const DefaultEventSize = 1024

type SelectorEvent uint8

const (
	_ SelectorEvent = iota
	SelectorEventRead
	SelectorEventWrite
	SelectorEventReadWrite
)

type PolledEvent struct {
	FD     int
	Events SelectorEvent
}

type Selector interface {
	Add(fd int, ev SelectorEvent) error
	Mod(fd int, ev SelectorEvent) error
	Del(fd int) error
	Wake() error
	Polling(ctx context.Context, timeoutMs int, handle func(evs []PolledEvent)) error
	Close() error
}
