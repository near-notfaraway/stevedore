package sd_session

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSessionStoresFileDescriptorsByAddressFamily(t *testing.T) {
	sess := &Session{key: sd_socket.SockaddrKey{}}
	fdStore, ok := any(sess).(interface {
		GetFDByFamily(int) (int, bool)
		SetFD(int, int) bool
		GetFDs() []int
	})
	require.True(t, ok)

	_, exists := fdStore.GetFDByFamily(unix.AF_INET)
	require.False(t, exists)
	_, exists = fdStore.GetFDByFamily(unix.AF_INET6)
	require.False(t, exists)

	require.True(t, fdStore.SetFD(unix.AF_INET, 10))
	require.True(t, fdStore.SetFD(unix.AF_INET6, 11))
	require.False(t, fdStore.SetFD(unix.AF_UNIX, 12))

	fd, exists := fdStore.GetFDByFamily(unix.AF_INET)
	require.True(t, exists)
	require.Equal(t, 10, fd)
	fd, exists = fdStore.GetFDByFamily(unix.AF_INET6)
	require.True(t, exists)
	require.Equal(t, 11, fd)
	require.Equal(t, []int{10, 11}, fdStore.GetFDs())
}
