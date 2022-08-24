package sd_session

import (
	"github.com/near-notfaraway/stevedore/sd_config"
	"golang.org/x/sys/unix"
	"sync"
	"time"
)

type Manager struct {
	recycleInterval time.Duration // time interval of recycle session
	timeoutSec      int64         // timeout for recycle session
	sessions        sync.Map      // map[string]*Session
}

func NewManager(config *sd_config.SessionConfig) *Manager {
	m := &Manager{
		recycleInterval: time.Second * time.Duration(config.RecycleIntervalSec),
		timeoutSec:      config.TimeoutSec,
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
			}
			return true
		})
	}
}

func (m *Manager) GetOrCreateSession(name string, sa unix.Sockaddr) (*Session, bool) {
	sess := NewSession(name, sa)
	actualSess, loaded := m.sessions.LoadOrStore(name, sess)

	if loaded {
		ss := actualSess.(*Session)
		ss.UpdateActive()
		return ss, loaded
	}

	return sess, loaded
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
