package ssh

import (
	"errors"
	"fmt"
	"testing"

	"github.com/fluid-movement/log-tui/config"
)

func TestConnectError_ErrorFormat(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	ce := &ConnectError{
		Host:   config.Host{Name: "web1"},
		Reason: FailUnreachable,
		Err:    inner,
	}
	got := ce.Error()
	want := "connect to web1: connection refused"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestConnectError_ErrorFormat_EmptyHostName(t *testing.T) {
	inner := fmt.Errorf("auth failed")
	ce := &ConnectError{
		Host:   config.Host{Name: ""},
		Reason: FailAuthFailed,
		Err:    inner,
	}
	got := ce.Error()
	// Should not panic; "Name" is empty string.
	want := "connect to : auth failed"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestConnectError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("timeout")
	ce := &ConnectError{
		Host:   config.Host{Name: "db1"},
		Reason: FailTimeout,
		Err:    inner,
	}
	if ce.Unwrap() != inner {
		t.Errorf("Unwrap() should return the inner error")
	}
}

func TestConnectError_ErrorsAs(t *testing.T) {
	inner := fmt.Errorf("unknown host")
	ce := &ConnectError{
		Host:   config.Host{Name: "srv"},
		Reason: FailHostKeyUnknown,
		Err:    inner,
	}
	// errors.As should find ConnectError when wrapped.
	wrapped := fmt.Errorf("outer: %w", ce)
	var target *ConnectError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find *ConnectError in chain")
	}
	if target.Reason != FailHostKeyUnknown {
		t.Errorf("Reason: got %v, want FailHostKeyUnknown", target.Reason)
	}
}

func TestConnectError_ErrorsIs(t *testing.T) {
	inner := fmt.Errorf("key changed")
	ce := &ConnectError{
		Host:   config.Host{Name: "srv"},
		Reason: FailHostKeyChanged,
		Err:    inner,
	}
	// errors.Is traverses the Unwrap chain.
	if !errors.Is(ce, inner) {
		t.Error("errors.Is should find inner error via Unwrap")
	}
}

func TestConnectError_NilInnerError(t *testing.T) {
	// ConnectError with nil Err should not panic in Error().
	ce := &ConnectError{
		Host:   config.Host{Name: "x"},
		Reason: FailUnreachable,
		Err:    nil,
	}
	// Should produce "connect to x: <nil>" without panicking.
	got := ce.Error()
	if got == "" {
		t.Error("Error() with nil Err should still return a non-empty string")
	}
}

func TestPort_DefaultsTo22(t *testing.T) {
	h := config.Host{Name: "h", Hostname: "1.2.3.4", Port: 0}
	if got := port(h); got != 22 {
		t.Errorf("port() with Port=0 should return 22, got %d", got)
	}
}

func TestPort_CustomPort(t *testing.T) {
	h := config.Host{Name: "h", Hostname: "1.2.3.4", Port: 2222}
	if got := port(h); got != 2222 {
		t.Errorf("port() with Port=2222 should return 2222, got %d", got)
	}
}

func TestIsAuthError_AuthFailureMessages(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain", true},
		{"no supported methods remain", true},
		{"connection refused", false},
		{"i/o timeout", false},
		{"", false},
	}
	for _, tc := range cases {
		var err error
		if tc.msg != "" {
			err = fmt.Errorf("%s", tc.msg)
		}
		got := isAuthError(err)
		if got != tc.want {
			t.Errorf("isAuthError(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}
