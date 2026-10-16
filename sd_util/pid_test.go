package sd_util

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"golang.org/x/sys/unix"
)

func TestAcquirePidFileConvey(t *testing.T) {
	convey.Convey("Test AcquirePidFile", t, func() {
		convey.Convey("locks the pid file and removes it on close", func() {
			path := filepath.Join(t.TempDir(), "stevedore.pid")

			owner, err := AcquirePidFile(path)
			convey.So(err, convey.ShouldBeNil)
			t.Cleanup(func() {
				_ = owner.Close()
			})

			pid, err := GetPid(path)
			convey.So(err, convey.ShouldBeNil)
			convey.So(pid, convey.ShouldEqual, os.Getpid())

			_, err = AcquirePidFile(path)
			convey.So(err, convey.ShouldNotBeNil)

			convey.So(owner.Close(), convey.ShouldBeNil)
			_, err = os.Stat(path)
			convey.So(os.IsNotExist(err), convey.ShouldBeTrue)
		})
	})
}

func TestGetRunningPidConvey(t *testing.T) {
	convey.Convey("Test GetRunningPid", t, func() {
		convey.Convey("returns the owner and ignores a stale unlocked pid", func() {
			path := filepath.Join(t.TempDir(), "stevedore.pid")
			owner, err := AcquirePidFile(path)
			convey.So(err, convey.ShouldBeNil)

			pid, err := GetRunningPid(path)
			convey.So(err, convey.ShouldBeNil)
			convey.So(pid, convey.ShouldEqual, os.Getpid())

			convey.So(owner.Close(), convey.ShouldBeNil)
			convey.So(os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644),
				convey.ShouldBeNil)
			pid, err = GetRunningPid(path)
			convey.So(err, convey.ShouldBeNil)
			convey.So(pid, convey.ShouldEqual, -1)
		})

		convey.Convey("trusts a locked pid file", func() {
			path := filepath.Join(t.TempDir(), "stevedore.pid")
			const lockedPID = 999999999
			convey.So(os.WriteFile(path, []byte(strconv.Itoa(lockedPID)), 0o644),
				convey.ShouldBeNil)
			owner, err := os.OpenFile(path, os.O_RDWR, 0)
			convey.So(err, convey.ShouldBeNil)
			convey.So(unix.Flock(int(owner.Fd()), unix.LOCK_EX|unix.LOCK_NB),
				convey.ShouldBeNil)
			defer owner.Close()

			pid, err := GetRunningPid(path)

			convey.So(err, convey.ShouldBeNil)
			convey.So(pid, convey.ShouldEqual, lockedPID)
		})
	})
}

func TestGetPidConvey(t *testing.T) {
	convey.Convey("Test GetPid", t, func() {
		convey.Convey("returns minus one for a missing file", func() {
			pid, err := GetPid(filepath.Join(t.TempDir(), "missing.pid"))
			convey.So(err, convey.ShouldBeNil)
			convey.So(pid, convey.ShouldEqual, -1)
		})

		convey.Convey("rejects invalid pid content", func() {
			path := filepath.Join(t.TempDir(), "invalid.pid")
			convey.So(os.WriteFile(path, []byte("abcd"), 0o600), convey.ShouldBeNil)

			pid, err := GetPid(path)

			convey.So(err, convey.ShouldNotBeNil)
			convey.So(pid, convey.ShouldEqual, -1)
		})

		convey.Convey("returns a valid pid", func() {
			path := filepath.Join(t.TempDir(), "valid.pid")
			convey.So(os.WriteFile(path, []byte("5555"), 0o600), convey.ShouldBeNil)

			pid, err := GetPid(path)

			convey.So(err, convey.ShouldBeNil)
			convey.So(pid, convey.ShouldEqual, 5555)
		})
	})
}
