package sd_session

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestNewWheelConvey(t *testing.T) {
	convey.Convey("Test NewWheel", t, func() {
		convey.Convey("normalizes dimensions", func() {
			wheel := NewWheel(0, -1, 10)
			convey.So(wheel.tickSec, convey.ShouldEqual, 1)
			convey.So(wheel.timeoutSec, convey.ShouldEqual, 0)
			convey.So(wheel.buckets, convey.ShouldHaveLength, 2)
			convey.So(wheel.currentTick, convey.ShouldEqual, 10)
		})
	})
}

func TestWheelAddConvey(t *testing.T) {
	convey.Convey("Test Wheel.Add", t, func() {
		convey.Convey("links a session into its expiry slot", func() {
			wheel := NewWheel(1, 5, 0)
			sess := &Session{}
			wheel.Add(sess, 2)
			convey.So(sess.inWheel, convey.ShouldBeTrue)
			convey.So(sess.expiresAt, convey.ShouldEqual, 8)
			convey.So(wheel.buckets[sess.wheelSlot].head, convey.ShouldEqual, sess)
		})
	})
}

func TestWheelTouchConvey(t *testing.T) {
	convey.Convey("Test Wheel.Touch", t, func() {
		convey.Convey("moves a session when its slot changes", func() {
			wheel := NewWheel(1, 2, 0)
			sess := &Session{}
			wheel.Add(sess, 0)
			oldSlot := sess.wheelSlot
			wheel.Touch(sess, 1)
			convey.So(sess.inWheel, convey.ShouldBeTrue)
			convey.So(sess.wheelSlot, convey.ShouldNotEqual, oldSlot)
			convey.So(sess.expiresAt, convey.ShouldEqual, 4)
		})
	})
}

func TestWheelRemoveConvey(t *testing.T) {
	convey.Convey("Test Wheel.Remove", t, func() {
		convey.Convey("unlinks a session", func() {
			wheel := NewWheel(1, 1, 0)
			sess := &Session{}
			wheel.Add(sess, 0)
			wheel.Remove(sess)
			convey.So(sess.inWheel, convey.ShouldBeFalse)
			convey.So(sess.wheelPrev, convey.ShouldBeNil)
			convey.So(sess.wheelNext, convey.ShouldBeNil)
		})
	})
}

func TestWheelAdvanceConvey(t *testing.T) {
	convey.Convey("Test Wheel.Advance", t, func() {
		convey.Convey("expires sessions up to the target time", func() {
			wheel := NewWheel(1, 1, 0)
			sess := &Session{}
			wheel.Add(sess, 0)
			var expired []*Session
			wheel.Advance(2, func(got *Session) {
				expired = append(expired, got)
			})
			convey.So(expired, convey.ShouldResemble, []*Session{sess})
			convey.So(wheel.currentTick, convey.ShouldEqual, 2)
		})
	})
}

func TestWheelExpiresAtConvey(t *testing.T) {
	convey.Convey("Test Wheel.expiresAt", t, func() {
		convey.Convey("preserves strict timeout semantics", func() {
			wheel := NewWheel(1, 5, 0)
			convey.So(wheel.expiresAt(10), convey.ShouldEqual, 16)
		})
	})
}

func TestWheelSlotForConvey(t *testing.T) {
	convey.Convey("Test Wheel.slotFor", t, func() {
		convey.Convey("rounds expiry to the next tick", func() {
			wheel := NewWheel(3, 6, 0)
			convey.So(wheel.slotFor(4), convey.ShouldEqual, 2)
		})
	})
}

func TestWheelInsertConvey(t *testing.T) {
	convey.Convey("Test Wheel.insert", t, func() {
		convey.Convey("prepends a session to a bucket", func() {
			wheel := NewWheel(1, 1, 0)
			first := &Session{}
			second := &Session{}
			wheel.insert(first, 0)
			wheel.insert(second, 0)
			convey.So(wheel.buckets[0].head, convey.ShouldEqual, second)
			convey.So(second.wheelNext, convey.ShouldEqual, first)
			convey.So(first.wheelPrev, convey.ShouldEqual, second)
		})
	})
}

func TestWheelExpireBucketConvey(t *testing.T) {
	convey.Convey("Test Wheel.expireBucket", t, func() {
		convey.Convey("reinserts sessions not yet due", func() {
			wheel := NewWheel(1, 3, 0)
			sess := &Session{expiresAt: 4}
			wheel.insert(sess, 1)
			called := false
			wheel.expireBucket(1, 2, func(*Session) {
				called = true
			})
			convey.So(called, convey.ShouldBeFalse)
			convey.So(sess.inWheel, convey.ShouldBeTrue)
			convey.So(wheel.buckets[wheel.slotFor(4)].head, convey.ShouldEqual, sess)
		})
	})
}
