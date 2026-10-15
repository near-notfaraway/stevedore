package sd_session

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/smartystreets/goconvey/convey"
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

func TestNewSessionConvey(t *testing.T) {
	convey.Convey("Test NewSession", t, func() {
		convey.Convey("stores the address key", func() {
			key := sd_socket.SockaddrKey{}
			sess := NewSession(key)
			convey.So(sess, convey.ShouldNotBeNil)
			convey.So(sess.key, convey.ShouldResemble, key)
		})
	})
}

func TestSessionGetKeyConvey(t *testing.T) {
	convey.Convey("Test Session.GetKey", t, func() {
		convey.Convey("returns the stored key", func() {
			key := sd_socket.SockaddrKey{}
			convey.So((&Session{key: key}).GetKey(), convey.ShouldResemble, key)
		})
	})
}

func TestSessionGetFDByFamilyConvey(t *testing.T) {
	convey.Convey("Test Session.GetFDByFamily", t, func() {
		convey.Convey("returns the descriptor for a supported family", func() {
			sess := &Session{fds: [2]int{10, 11}, fdSet: [2]bool{false, true}}
			fd, ok := sess.GetFDByFamily(unix.AF_INET6)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(fd, convey.ShouldEqual, 11)
		})
	})
}

func TestSessionSetFDConvey(t *testing.T) {
	convey.Convey("Test Session.SetFD", t, func() {
		convey.Convey("stores a supported descriptor", func() {
			sess := &Session{}
			convey.So(sess.SetFD(unix.AF_INET, 10), convey.ShouldBeTrue)
			convey.So(sess.fds[0], convey.ShouldEqual, 10)
			convey.So(sess.fdSet[0], convey.ShouldBeTrue)
		})
	})
}

func TestSessionGetFDsConvey(t *testing.T) {
	convey.Convey("Test Session.GetFDs", t, func() {
		convey.Convey("returns only initialized descriptors", func() {
			sess := &Session{fds: [2]int{10, 11}, fdSet: [2]bool{true, false}}
			convey.So(sess.GetFDs(), convey.ShouldResemble, []int{10})
		})
	})
}

func TestFdFamilyIndexConvey(t *testing.T) {
	convey.Convey("Test fdFamilyIndex", t, func() {
		convey.Convey("maps IPv4 and IPv6", func() {
			index, ok := fdFamilyIndex(unix.AF_INET)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(index, convey.ShouldEqual, 0)

			index, ok = fdFamilyIndex(unix.AF_INET6)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(index, convey.ShouldEqual, 1)

			_, ok = fdFamilyIndex(unix.AF_UNIX)
			convey.So(ok, convey.ShouldBeFalse)
		})
	})
}
