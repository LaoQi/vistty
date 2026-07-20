package drm

import (
	"testing"
	"time"
)

// TestDRMSurface_SetActiveFalseClearsFlipPending 验证 P1-23 修复（dumb 路径）：
// VT 切走时 SetActive(false) 必须清 flipPending 并排空 flipCh。否则 DropMaster
// 后内核不再发送 flip 事件，切回后首次 Swap 的 waitForFlip 必然走完 5s 超时。
func TestDRMSurface_SetActiveFalseClearsFlipPending(t *testing.T) {
	s := &DRMSurface{
		active:      true,
		flipCh:      make(chan struct{}, 1),
		flipPending: true,
		done:        make(chan struct{}),
	}
	// 模拟上一帧 flip 事件已到但 Swap 未消费（残留信号）
	s.flipCh <- struct{}{}

	s.SetActive(false)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		t.Error("SetActive(false) 后 active 应为 false")
	}
	if s.flipPending {
		t.Error("SetActive(false) 后 flipPending 应清零")
	}
	// flipCh 应被排空
	select {
	case <-s.flipCh:
		t.Error("flipCh 残留信号应被排空")
	default:
	}
}

// TestDRMSurface_SetActiveTrueNoFlipPendingChange 验证 SetActive(true)
// 不触碰 flipPending / flipCh（仅置位 active 标志）。
func TestDRMSurface_SetActiveTrueNoFlipPendingChange(t *testing.T) {
	s := &DRMSurface{
		active:      false,
		flipCh:      make(chan struct{}, 1),
		flipPending: true,
		done:        make(chan struct{}),
	}

	s.SetActive(true)

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		t.Error("SetActive(true) 后 active 应为 true")
	}
	if !s.flipPending {
		t.Error("SetActive(true) 不应清 flipPending")
	}
}

// TestDRMSurface_SetActiveFalseNoPendingNoop 验证无 pending flip 时
// SetActive(false) 不出错（flipPending=false 路径）。
func TestDRMSurface_SetActiveFalseNoPendingNoop(t *testing.T) {
	s := &DRMSurface{
		active:      true,
		flipCh:      make(chan struct{}, 1),
		flipPending: false,
		done:        make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		s.SetActive(false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SetActive(false) 无 pending 时不应阻塞")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		t.Error("SetActive(false) 后 active 应为 false")
	}
}
