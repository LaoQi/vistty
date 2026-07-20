package gbm

import (
	"sync"
	"testing"
	"time"
)

// newSyncTestSurface 构造仅含同步字段的 GBMSurface，用于测试 flip 等待逻辑，
// 不依赖 EGL/GBM/GL 硬件。flipTimeout 设为短值加速测试。
func newSyncTestSurface() *GBMSurface {
	return &GBMSurface{
		flipDone:    make(chan struct{}, 1),
		closedCh:    make(chan struct{}),
		flipTimeout: 50 * time.Millisecond,
	}
}

// TestWaitForFlipComplete_NoPending 验证无 pending flip 时立即返回 true。
func TestWaitForFlipComplete_NoPending(t *testing.T) {
	s := newSyncTestSurface()
	start := time.Now()
	ok := s.waitForFlipComplete()
	elapsed := time.Since(start)
	if !ok {
		t.Error("expected true when no flip pending")
	}
	if elapsed > 10*time.Millisecond {
		t.Errorf("should return immediately, took %v", elapsed)
	}
}

// TestWaitForFlipComplete_Timeout 复现"flip 事件丢失导致 Swap 永久阻塞"的核心缺陷。
//
// 修复前：sync.Cond.Wait() 无超时，flipPending 永真时 Swap 永久阻塞。
// 修复后：time.After 超时返回 true，渲染不卡死，且 flipPending 被清零。
func TestWaitForFlipComplete_Timeout(t *testing.T) {
	s := newSyncTestSurface()
	s.flipPending = true

	start := time.Now()
	ok := s.waitForFlipComplete()
	elapsed := time.Since(start)

	if !ok {
		t.Error("超时应返回 true（跳过本帧），不能因超时误判为关闭")
	}
	if elapsed < s.flipTimeout {
		t.Errorf("应等待约 %v 才超时，实际 %v", s.flipTimeout, elapsed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("超时后不应长时间阻塞，实际 %v（修复前会永久阻塞）", elapsed)
	}

	s.commitMu.Lock()
	stillPending := s.flipPending
	s.commitMu.Unlock()
	if stillPending {
		t.Error("超时后 flipPending 应清零，否则下一帧仍会等待")
	}
}

// TestWaitForFlipTimeoutReleasesCommitted 验证 P0-8 修复：超时分支必须模拟
// onFlipComplete 轮转三缓冲，避免下一帧 Swap 的 `s.committed = frame`
// 覆盖旧 committed 导致 BO+FB 泄漏。
func TestWaitForFlipTimeoutReleasesCommitted(t *testing.T) {
	s := newSyncTestSurface()
	oldScanout := &pendingFrame{bo: 1, fbID: 100, stride: 320}
	newCommitted := &pendingFrame{bo: 2, fbID: 200, stride: 320}
	s.scanout = oldScanout
	s.committed = newCommitted
	s.flipPending = true

	start := time.Now()
	ok := s.waitForFlipComplete()
	elapsed := time.Since(start)

	if !ok {
		t.Error("超时应返回 true")
	}
	if elapsed < s.flipTimeout {
		t.Errorf("应等待约 %v 才超时，实际 %v", s.flipTimeout, elapsed)
	}

	s.commitMu.Lock()
	defer s.commitMu.Unlock()
	if s.committed != nil {
		t.Error("超时后 committed 应清空，否则下一帧 Swap 覆盖会泄漏 BO+FB")
	}
	if s.scanout != newCommitted {
		t.Error("超时后 scanout 应为原 committed（模拟 flip 完成轮转）")
	}
	if s.releaseBO != oldScanout {
		t.Error("超时后 releaseBO 应为原 scanout，下次 Swap 开头释放")
	}
	if s.flipPending {
		t.Error("超时后 flipPending 应清零")
	}
}

// TestWaitForFlipComplete_OnFlipDone 验证 flip 完成事件正常到达时不超时。
func TestWaitForFlipComplete_OnFlipDone(t *testing.T) {
	s := newSyncTestSurface()
	s.flipPending = true

	go func() {
		time.Sleep(10 * time.Millisecond)
		s.onFlipComplete()
	}()

	start := time.Now()
	ok := s.waitForFlipComplete()
	elapsed := time.Since(start)

	if !ok {
		t.Error("flip 完成后应返回 true")
	}
	if elapsed >= s.flipTimeout {
		t.Errorf("flip 完成应早于超时，实际 %v", elapsed)
	}
}

// TestWaitForFlipComplete_OnClose 验证 Close 能唤醒等待中的 Swap。
//
// 修复前：onFlipComplete 在 closed 时不 Signal，依赖 Close 的 Signal 时序。
// 修复后：Close 通过 close(closedCh) 可靠唤醒所有等待者。
func TestWaitForFlipComplete_OnClose(t *testing.T) {
	s := newSyncTestSurface()
	s.flipPending = true

	go func() {
		time.Sleep(10 * time.Millisecond)
		s.commitMu.Lock()
		s.closed = true
		s.commitMu.Unlock()
		close(s.closedCh)
	}()

	start := time.Now()
	ok := s.waitForFlipComplete()
	elapsed := time.Since(start)

	if ok {
		t.Error("Close 后应返回 false")
	}
	if elapsed >= s.flipTimeout {
		t.Errorf("Close 唤醒应早于超时，实际 %v", elapsed)
	}
}

// TestOnFlipComplete_StateTransition 验证 onFlipComplete 的三缓冲状态流转。
// scanout→releaseBO, committed→scanout, committed=nil, flipPending=false。
func TestOnFlipComplete_StateTransition(t *testing.T) {
	s := newSyncTestSurface()
	oldScanout := &pendingFrame{bo: 1, fbID: 100, stride: 320}
	newCommitted := &pendingFrame{bo: 2, fbID: 200, stride: 320}
	s.scanout = oldScanout
	s.committed = newCommitted
	s.flipPending = true

	s.onFlipComplete()

	s.commitMu.Lock()
	defer s.commitMu.Unlock()
	if s.releaseBO != oldScanout {
		t.Error("releaseBO 应为旧 scanout")
	}
	if s.scanout != newCommitted {
		t.Error("scanout 应为旧 committed")
	}
	if s.committed != nil {
		t.Error("committed 应清空")
	}
	if s.flipPending {
		t.Error("flipPending 应清零")
	}
	// flipDone 应收到信号
	select {
	case <-s.flipDone:
	default:
		t.Error("onFlipComplete 应发送 flipDone 信号")
	}
}

// TestOnFlipComplete_ClosedNoSignal 验证 closed 后 onFlipComplete 不改状态。
func TestOnFlipComplete_ClosedNoSignal(t *testing.T) {
	s := newSyncTestSurface()
	s.scanout = &pendingFrame{bo: 1, fbID: 100, stride: 320}
	s.committed = &pendingFrame{bo: 2, fbID: 200, stride: 320}
	s.commitMu.Lock()
	s.flipPending = true
	s.closed = true
	s.commitMu.Unlock()

	s.onFlipComplete()

	s.commitMu.Lock()
	defer s.commitMu.Unlock()
	if s.scanout == nil || s.scanout.bo != 1 {
		t.Error("closed 时 scanout 不应变")
	}
	if s.committed == nil || s.committed.bo != 2 {
		t.Error("closed 时 committed 不应变")
	}
	if !s.flipPending {
		t.Error("closed 时 flipPending 不应变")
	}
	select {
	case <-s.flipDone:
		t.Error("closed 时不应发送 flipDone（由 closedCh 兜底唤醒）")
	default:
	}
}

// TestResidualFlipDoneDrained 验证残留 flipDone 信号在无 pending 时被 drain，
// 防止下一帧误唤醒。
func TestResidualFlipDoneDrained(t *testing.T) {
	s := newSyncTestSurface()
	// 模拟上一帧 onFlipComplete 发了信号但 Swap 未消费
	s.flipDone <- struct{}{}
	s.flipPending = false

	ok := s.waitForFlipComplete()
	if !ok {
		t.Error("无 pending 应返回 true")
	}
	// 残留信号应被 drain
	select {
	case <-s.flipDone:
		t.Error("残留信号应被 drain，不应残留")
	default:
	}
}

// TestConcurrentSwapAndFlip 模拟渲染主线程 Swap 等待与 eventLoop onFlipComplete
// 并发交互，验证不发生死锁或数据竞争。
func TestConcurrentSwapAndFlip(t *testing.T) {
	s := newSyncTestSurface()
	s.flipTimeout = 200 * time.Millisecond

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			s.commitMu.Lock()
			s.flipPending = true
			s.commitMu.Unlock()
			go func() {
				time.Sleep(time.Millisecond)
				s.onFlipComplete()
			}()
			s.waitForFlipComplete()
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("并发 Swap/flip 死锁（修复前 sync.Cond 无超时会永久阻塞）")
	}
}

// TestSetActiveFalseRotatesBuffers 验证 P1-23 修复：VT 切走时
// GBMSurface.SetActive(false) 必须模拟 onFlipComplete 轮转三缓冲并清
// flipPending。否则 DropMaster 后内核不再发送 flip 事件，切回后首次
// Swap 必然走完 5s 超时；且 committed 帧会被下一帧 Swap 覆盖泄漏。
func TestSetActiveFalseRotatesBuffers(t *testing.T) {
	s := newSyncTestSurface()
	oldScanout := &pendingFrame{bo: 1, fbID: 100, stride: 320}
	newCommitted := &pendingFrame{bo: 2, fbID: 200, stride: 320}
	s.scanout = oldScanout
	s.committed = newCommitted
	s.flipPending = true
	s.active = true

	s.SetActive(false)

	s.commitMu.Lock()
	defer s.commitMu.Unlock()
	if s.active {
		t.Error("SetActive(false) 后 active 应为 false")
	}
	if s.flipPending {
		t.Error("SetActive(false) 后 flipPending 应清零")
	}
	if s.committed != nil {
		t.Error("SetActive(false) 后 committed 应清空（避免下帧 Swap 覆盖泄漏）")
	}
	if s.scanout != newCommitted {
		t.Error("SetActive(false) 后 scanout 应为原 committed")
	}
	if s.releaseBO != oldScanout {
		t.Error("SetActive(false) 后 releaseBO 应为原 scanout")
	}
}

// TestSetActiveTrueNoRotation 验证 SetActive(true) 不触碰三缓冲状态
// （仅重新置位 active 标志），避免在切回时误清尚未释放的帧。
func TestSetActiveTrueNoRotation(t *testing.T) {
	s := newSyncTestSurface()
	oldScanout := &pendingFrame{bo: 1, fbID: 100, stride: 320}
	newCommitted := &pendingFrame{bo: 2, fbID: 200, stride: 320}
	s.scanout = oldScanout
	s.committed = newCommitted
	s.flipPending = true
	s.active = false

	s.SetActive(true)

	s.commitMu.Lock()
	defer s.commitMu.Unlock()
	if !s.active {
		t.Error("SetActive(true) 后 active 应为 true")
	}
	if !s.flipPending {
		t.Error("SetActive(true) 不应清 flipPending")
	}
	if s.scanout != oldScanout {
		t.Error("SetActive(true) 不应改 scanout")
	}
	if s.committed != newCommitted {
		t.Error("SetActive(true) 不应改 committed")
	}
	if s.releaseBO != nil {
		t.Error("SetActive(true) 不应改 releaseBO")
	}
}

// TestGBMSurfaceCloseRemovesFromDeviceMap 验证 GBMSurface.Close 从
// GBMDevice.surfaces map 移除自身（P2-32）。
func TestGBMSurfaceCloseRemovesFromDeviceMap(t *testing.T) {
	d := &GBMDevice{
		surfaces: make(map[uint32]*GBMSurface),
	}
	s := &GBMSurface{
		device:   d,
		crtcID:   42,
		flipDone: make(chan struct{}, 1),
		closedCh: make(chan struct{}),
		commitMu: sync.Mutex{},
	}
	d.surfaces[42] = s

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	d.mu.Lock()
	_, ok := d.surfaces[42]
	d.mu.Unlock()
	if ok {
		t.Error("surface should be removed from device.surfaces after Close")
	}
}
