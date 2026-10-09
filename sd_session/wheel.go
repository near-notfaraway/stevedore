package sd_session

// wheelBucket 是侵入式链表的桶头；链表节点直接复用 Session 中的 wheelPrev/wheelNext。
type wheelBucket struct {
	head *Session
}

// Wheel 由所属 Reactor 单 goroutine 独占，无需锁。
// 桶数量覆盖一个完整超时周期，Session 依据绝对过期时间落入对应槽位。
type Wheel struct {
	tickSec     int64         // 每个时间轮 tick 代表的秒数
	timeoutSec  int64         // Add 和 Touch 使用的空闲超时时间
	currentTick int64         // 最近一次处理的绝对 tick
	buckets     []wheelBucket // Session 侵入式链表桶
}

// NewWheel 创建固定尺寸时间轮，多保留一个槽位以区分当前轮次和下一轮次。
func NewWheel(tickSec, timeoutSec, now int64) *Wheel {
	if tickSec < 1 {
		tickSec = 1
	}
	if timeoutSec < 0 {
		timeoutSec = 0
	}
	slotCount := int((timeoutSec+tickSec-1)/tickSec) + 1
	if slotCount < 2 {
		slotCount = 2
	}
	return &Wheel{
		tickSec:     tickSec,
		timeoutSec:  timeoutSec,
		currentTick: now / tickSec,
		buckets:     make([]wheelBucket, slotCount),
	}
}

// Add 将新 Session 加入时间轮；重复加入时先解除旧链表关系。
func (w *Wheel) Add(sess *Session, now int64) {
	if sess.inWheel {
		w.Remove(sess)
	}
	sess.expiresAt = w.expiresAt(now)
	w.insert(sess, w.slotFor(sess.expiresAt))
}

// Touch 延长 Session 生命周期。同槽续期仅更新时间，跨槽时才移动链表节点。
func (w *Wheel) Touch(sess *Session, now int64) {
	expiresAt := w.expiresAt(now)
	slot := w.slotFor(expiresAt)
	sess.expiresAt = expiresAt
	if sess.inWheel && sess.wheelSlot == slot {
		return
	}
	if sess.inWheel {
		w.Remove(sess)
	}
	w.insert(sess, slot)
}

// Remove 将 Session 从时间轮解除。未加入时间轮时该操作为空操作。
func (w *Wheel) Remove(sess *Session) {
	if !sess.inWheel {
		return
	}
	bucket := &w.buckets[sess.wheelSlot]
	if sess.wheelPrev == nil {
		bucket.head = sess.wheelNext
	} else {
		sess.wheelPrev.wheelNext = sess.wheelNext
	}
	if sess.wheelNext != nil {
		sess.wheelNext.wheelPrev = sess.wheelPrev
	}
	sess.wheelPrev = nil
	sess.wheelNext = nil
	sess.inWheel = false
}

// Advance 推进到 now 所在的绝对 tick，并回调所有真正到期的 Session。
// 若时钟一次跨越超过一整轮，只需扫描每个桶一次，避免逐 tick 追赶。
func (w *Wheel) Advance(now int64, expire func(*Session)) {
	targetTick := now / w.tickSec
	if targetTick <= w.currentTick {
		return
	}

	if skipped := targetTick - w.currentTick; skipped > int64(len(w.buckets)) {
		w.currentTick = targetTick - int64(len(w.buckets))
	}
	for w.currentTick < targetTick {
		w.currentTick++
		w.expireBucket(int(w.currentTick%int64(len(w.buckets))), now, expire)
	}
}

// expiresAt 保持旧实现 now-lastActive > timeoutSec 的严格大于语义。
func (w *Wheel) expiresAt(now int64) int64 {
	return now + w.timeoutSec + 1
}

// slotFor 使用向上取整的绝对 tick，确保 Session 不会在期限前进入到期桶。
func (w *Wheel) slotFor(expiresAt int64) int {
	expireTick := (expiresAt + w.tickSec - 1) / w.tickSec
	return int(expireTick % int64(len(w.buckets)))
}

// insert 将 Session 插入目标桶头，不产生额外链表节点分配。
func (w *Wheel) insert(sess *Session, slot int) {
	bucket := &w.buckets[slot]
	sess.wheelSlot = slot
	sess.wheelPrev = nil
	sess.wheelNext = bucket.head
	sess.inWheel = true
	if bucket.head != nil {
		bucket.head.wheelPrev = sess
	}
	bucket.head = sess
}

// expireBucket 先摘下整个桶再遍历，使尚未到期的 Session 可安全插回任意桶，
// 包括当前槽位，而不会干扰本轮遍历。
func (w *Wheel) expireBucket(slot int, now int64, expire func(*Session)) {
	sess := w.buckets[slot].head
	w.buckets[slot].head = nil
	for sess != nil {
		next := sess.wheelNext
		sess.wheelPrev = nil
		sess.wheelNext = nil
		sess.inWheel = false

		if sess.expiresAt <= now {
			expire(sess)
		} else {
			w.insert(sess, w.slotFor(sess.expiresAt))
		}
		sess = next
	}
}
