package ssh

import (
	"testing"
	"time"
)

func TestReconnectDelay_KnownAttempts(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 2 * time.Second},
		{1, 4 * time.Second},
		{2, 8 * time.Second},
		{3, 30 * time.Second},
	}
	for _, tc := range cases {
		got := reconnectDelay(tc.attempt)
		if got != tc.want {
			t.Errorf("reconnectDelay(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

func TestReconnectDelay_Cap(t *testing.T) {
	// Any attempt >= len(reconnectDelays) should be capped at the last value (30s).
	cap := 30 * time.Second
	for _, attempt := range []int{4, 10, 100, 1000} {
		got := reconnectDelay(attempt)
		if got != cap {
			t.Errorf("reconnectDelay(%d) = %v, want %v (cap)", attempt, got, cap)
		}
	}
}

func TestReconnectDelay_Negative(t *testing.T) {
	// Negative attempts should not panic and should return the first delay (2s).
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("reconnectDelay(-1) panicked: %v", r)
		}
	}()
	got := reconnectDelay(-1)
	want := 2 * time.Second
	if got != want {
		t.Errorf("reconnectDelay(-1) = %v, want %v", got, want)
	}
}

func TestReconnectDelays_Monotonic(t *testing.T) {
	// Each delay should be >= the previous (monotonically non-decreasing).
	for i := 1; i < len(reconnectDelays); i++ {
		if reconnectDelays[i] < reconnectDelays[i-1] {
			t.Errorf("reconnectDelays[%d]=%v < reconnectDelays[%d]=%v — not monotonic",
				i, reconnectDelays[i], i-1, reconnectDelays[i-1])
		}
	}
}
