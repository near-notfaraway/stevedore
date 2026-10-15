package sd_session

import (
	"github.com/near-notfaraway/stevedore/sd_socket"
	"golang.org/x/sys/unix"
)

// Session 同时作为 Reactor Session 表中的值和时间轮中的侵入式链表节点。
// 它只允许由所属 Reactor goroutine 访问，因此字段不需要锁或原子操作。
type Session struct {
	key       sd_socket.SockaddrKey // 所属 Reactor 中的下游地址键
	fds       [2]int                // IPv4、IPv6 上游 socket fd
	fdSet     [2]bool               // 对应地址族的 fd 是否已创建
	expiresAt int64                 // 绝对过期时间，单位秒
	wheelSlot int                   // 当前所在的时间轮槽位
	wheelPrev *Session              // 桶内前一个 Session
	wheelNext *Session              // 桶内后一个 Session
	inWheel   bool                  // 是否已链接到时间轮
}

func NewSession(key sd_socket.SockaddrKey) *Session {
	return &Session{key: key}
}

func (s *Session) GetKey() sd_socket.SockaddrKey {
	return s.key
}

func (s *Session) GetFDByFamily(family int) (int, bool) {
	index, ok := fdFamilyIndex(family)
	if !ok || !s.fdSet[index] {
		return 0, false
	}
	return s.fds[index], true
}

func (s *Session) SetFD(family, fd int) bool {
	index, ok := fdFamilyIndex(family)
	if !ok {
		return false
	}
	s.fds[index] = fd
	s.fdSet[index] = true
	return true
}

func (s *Session) GetFDs() []int {
	fds := make([]int, 0, len(s.fds))
	for i, fd := range s.fds {
		if s.fdSet[i] {
			fds = append(fds, fd)
		}
	}
	return fds
}

func fdFamilyIndex(family int) (int, bool) {
	switch family {
	case unix.AF_INET:
		return 0, true
	case unix.AF_INET6:
		return 1, true
	default:
		return 0, false
	}
}
