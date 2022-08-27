package sd_session

import (
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_selector"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"golang.org/x/sys/unix"
	"sync"
	"time"
)

type Manager struct {
	recycleInterval time.Duration        // time interval of recycle session
	timeoutSec      int64                // timeout for recycle session
	sessions        sync.Map             // map[string]*Session
	evChanPool      sync.Pool            // allocate event chan
	selector        sd_selector.Selector // unregister fd when recycle
}

func NewManager(config *sd_config.SessionConfig, evChanPool sync.Pool, selector sd_selector.Selector) *Manager {
	m := &Manager{
		recycleInterval: time.Second * time.Duration(config.RecycleIntervalSec),
		timeoutSec:      config.TimeoutSec,
		evChanPool:      evChanPool,
		selector:		 selector,
	}
	go m.sessionRecycle()

	return m
}

func (m *Manager) sessionRecycle() {
	tick := time.NewTicker(m.recycleInterval)
	for range tick.C {
		m.sessions.Range(func(k, v interface{}) bool {
			key := k.(string)
			sess := v.(*Session)
			if time.Now().Unix()-sess.LastActive() > m.timeoutSec {
				m.sessions.Delete(key)
				_ = m.selector.Del(sess.fd)
				m.evChanPool.Put(sess.ch)
			}
			return true
		})
	}
}

func (m *Manager) GetOrCreateSession(name string, sa unix.Sockaddr) (*Session, bool, error) {
	sess := NewSession(name, sa)
	actualSess, loaded := m.sessions.LoadOrStore(name, sess)

	if loaded {
		ss := actualSess.(*Session)
		ss.UpdateActive()
		return ss, loaded, nil
	}

	fd, err := sd_socket.UDPSocket(unix.AF_INET, true, false, false)
	if err != nil {
		return nil, false, fmt.Errorf("create socket failed: %w", err)
	}
	sess.fd = fd
	sess.ch = m.evChanPool.Get().(chan struct{})

	return sess, loaded, nil
}

func (m *Manager) GetSession(name string) *Session {
	v, ok := m.sessions.Load(name)
	if ok {
		sess := v.(*Session)
		sess.UpdateActive()
		return sess
	}
	return nil
}
