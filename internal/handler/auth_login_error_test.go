package handler

import (
	"net/http"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestLoginFailureAppError_InvalidCredentials(t *testing.T) {
	// Both the legacy message and the current one must map to
	// user.invalid_credentials (identifier can be username or email).
	for _, msg := range []string{
		"Invalid email or password",
		"Invalid username/email or password",
	} {
		appErr := loginFailureAppError(&types.LoginResponse{Message: msg})
		if appErr == nil {
			t.Fatalf("loginFailureAppError(%q) returned nil", msg)
		}
		if appErr.HTTPCode != http.StatusUnauthorized {
			t.Fatalf("HTTPCode = %d, want %d (msg %q)", appErr.HTTPCode, http.StatusUnauthorized, msg)
		}
		details, ok := appErr.Details.(map[string]string)
		if !ok {
			t.Fatalf("details type = %T, want map[string]string (msg %q)", appErr.Details, msg)
		}
		if got := details["reason"]; got != "user.invalid_credentials" {
			t.Fatalf("reason = %q, want %q (msg %q)", got, "user.invalid_credentials", msg)
		}
	}
}

func TestLoginFailureAppError_AccountDisabled(t *testing.T) {
	appErr := loginFailureAppError(&types.LoginResponse{Message: "Account is disabled"})
	if appErr == nil {
		t.Fatal("loginFailureAppError returned nil")
	}
	if appErr.HTTPCode != http.StatusForbidden {
		t.Fatalf("HTTPCode = %d, want %d", appErr.HTTPCode, http.StatusForbidden)
	}
	details, ok := appErr.Details.(map[string]string)
	if !ok {
		t.Fatalf("details type = %T, want map[string]string", appErr.Details)
	}
	if got := details["reason"]; got != "user.account_disabled" {
		t.Fatalf("reason = %q, want %q", got, "user.account_disabled")
	}
}

func TestRegistrationFailureAppError_EmailExists(t *testing.T) {
	appErr := registrationFailureAppError(assertErrorString("user with this email already exists"))
	if appErr == nil {
		t.Fatal("registrationFailureAppError returned nil")
	}
	if appErr.HTTPCode != http.StatusConflict {
		t.Fatalf("HTTPCode = %d, want %d", appErr.HTTPCode, http.StatusConflict)
	}
	details, ok := appErr.Details.(map[string]string)
	if !ok {
		t.Fatalf("details type = %T, want map[string]string", appErr.Details)
	}
	if got := details["reason"]; got != "user.email_exists" {
		t.Fatalf("reason = %q, want %q", got, "user.email_exists")
	}
}

type assertErrorString string

func (e assertErrorString) Error() string {
	return string(e)
}
