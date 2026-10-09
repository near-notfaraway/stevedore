package sd_session

import (
	"github.com/near-notfaraway/stevedore/sd_socket"
	"golang.org/x/sys/unix"
)

// Session 同时作为 Reactor Session 表中的值和时间轮中的侵入式链表节点。
// 它只允许由所属 Reactor goroutine 访问，因此字段不需要锁或原子操作。
type Session struct {
	key       sd_socket.SockaddrKey // 所属 Reactor 中的下游地址键
	fd        int                   // 与上游通信的 socket fd
	expiresAt int64                 // 绝对过期时间，单位秒
	wheelSlot int                   // 当前所在的时间轮槽位
	wheelPrev *Session              // 桶内前一个 Session
	wheelNext *Session              // 桶内后一个 Session
	inWheel   bool                  // 是否已链接到时间轮
}

func NewSession(key sd_socket.SockaddrKey, fd int) *Session {
	return &Session{
		key: key,
		fd:  fd,
	}
}

func (s *Session) GetKey() sd_socket.SockaddrKey {
	return s.key
}

func (s *Session) GetFD() int {
	return s.fd
}

func (s *Session) Close() error {
	return unix.Close(s.fd)
}
