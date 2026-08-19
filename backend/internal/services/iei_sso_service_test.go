package services

import (
	"errors"
	"testing"
	"time"

	"clawreef/internal/config"
)

func testIEISystemConfig() config.IEISystemConfig {
	return config.IEISystemConfig{
		Enabled:       true,
		AESKey:        "TESTKEY123456789",
		AESIV:         "0123456789ABCDEF",
		TokenTTL:      30 * time.Second,
		SessionTTL:    30 * time.Minute,
		SessionSecret: "test-only-iei-session-secret-at-least-32-bytes",
		Timezone:      "Asia/Shanghai",
	}
}

func TestIEISSOServiceExchangesDocumentCompatibleAESCBCToken(t *testing.T) {
	service, err := NewIEISSOService(testIEISystemConfig())
	if err != nil {
		t.Fatalf("NewIEISSOService() error = %v", err)
	}
	// Independently generated with Node/OpenSSL-compatible AES-128-CBC and
	// PKCS padding for Owner+Tag@Example.COM+2026-08-18 12:34:56.
	const externalToken = "xzOmdmY7dW9qI52/OrpGAcQFG1IPcJjc32hUjq/FfHCEiOdeJOJu01UGKEcw46pb"
	location, _ := time.LoadLocation("Asia/Shanghai")
	service.now = func() time.Time {
		return time.Date(2026, 8, 18, 12, 35, 10, 0, location)
	}

	session, err := service.ExchangeExternalToken(externalToken)
	if err != nil {
		t.Fatalf("ExchangeExternalToken() error = %v", err)
	}
	if session.Email != "owner+tag@example.com" {
		t.Fatalf("session email = %q", session.Email)
	}
	validated, err := service.ValidateSession(session.Token)
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	if validated.Email != session.Email || validated.SessionID != session.SessionID {
		t.Fatalf("validated session = %#v, issued = %#v", validated, session)
	}
}

func TestIEISSOServiceRejectsExpiredOrTamperedExternalToken(t *testing.T) {
	service, err := NewIEISSOService(testIEISystemConfig())
	if err != nil {
		t.Fatalf("NewIEISSOService() error = %v", err)
	}
	const externalToken = "xzOmdmY7dW9qI52/OrpGAcQFG1IPcJjc32hUjq/FfHCEiOdeJOJu01UGKEcw46pb"
	location, _ := time.LoadLocation("Asia/Shanghai")
	service.now = func() time.Time {
		return time.Date(2026, 8, 18, 12, 36, 0, 0, location)
	}
	if _, err := service.ExchangeExternalToken(externalToken); !errors.Is(err, ErrInvalidIEISystemToken) {
		t.Fatalf("expired token error = %v", err)
	}
	service.now = func() time.Time {
		return time.Date(2026, 8, 18, 12, 35, 10, 0, location)
	}
	if _, err := service.ExchangeExternalToken(externalToken[:len(externalToken)-1] + "A"); !errors.Is(err, ErrInvalidIEISystemToken) {
		t.Fatalf("tampered token error = %v", err)
	}
}

func TestIEISSOServiceRejectsInvalidConfiguration(t *testing.T) {
	cfg := testIEISystemConfig()
	cfg.AESIV = "short"
	if _, err := NewIEISSOService(cfg); err == nil {
		t.Fatal("NewIEISSOService() accepted a non-16-byte IV")
	}
	cfg = testIEISystemConfig()
	cfg.TokenTTL = 31 * time.Second
	if _, err := NewIEISSOService(cfg); err == nil {
		t.Fatal("NewIEISSOService() accepted a token TTL above 30 seconds")
	}
	cfg = testIEISystemConfig()
	cfg.SessionSecret = "short"
	if _, err := NewIEISSOService(cfg); err == nil {
		t.Fatal("NewIEISSOService() accepted a short session secret")
	}
}
