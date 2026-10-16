package sd_session

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/smartystreets/goconvey/convey"
	"golang.org/x/sys/unix"
)

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
			sess := &Session{fds: [2]int{10, 11}, fdSet: [2]bool{true, true}}
			fd, ok := sess.GetFDByFamily(unix.AF_INET)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(fd, convey.ShouldEqual, 10)

			fd, ok = sess.GetFDByFamily(unix.AF_INET6)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(fd, convey.ShouldEqual, 11)
		})

		convey.Convey("reports unset descriptors", func() {
			sess := &Session{}
			_, ok := sess.GetFDByFamily(unix.AF_INET)
			convey.So(ok, convey.ShouldBeFalse)
			_, ok = sess.GetFDByFamily(unix.AF_INET6)
			convey.So(ok, convey.ShouldBeFalse)
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

		convey.Convey("stores IPv6 and rejects unsupported families", func() {
			sess := &Session{}
			convey.So(sess.SetFD(unix.AF_INET6, 11), convey.ShouldBeTrue)
			convey.So(sess.fds[1], convey.ShouldEqual, 11)
			convey.So(sess.fdSet[1], convey.ShouldBeTrue)
			convey.So(sess.SetFD(unix.AF_UNIX, 12), convey.ShouldBeFalse)
		})
	})
}

func TestSessionGetFDsConvey(t *testing.T) {
	convey.Convey("Test Session.GetFDs", t, func() {
		convey.Convey("returns only initialized descriptors", func() {
			sess := &Session{fds: [2]int{10, 11}, fdSet: [2]bool{true, false}}
			convey.So(sess.GetFDs(), convey.ShouldResemble, []int{10})
		})

		convey.Convey("returns descriptors for both address families", func() {
			sess := &Session{fds: [2]int{10, 11}, fdSet: [2]bool{true, true}}
			convey.So(sess.GetFDs(), convey.ShouldResemble, []int{10, 11})
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
