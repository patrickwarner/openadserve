package token

import (
	"testing"
	"time"
)

func TestGenerateVerify(t *testing.T) {
	secret := []byte("secret")
	tok, err := Generate("r1", "i1", "c1", "cid1", "li1", "u1", "1", secret)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	p, err := Verify(tok, secret, time.Minute)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if p.RequestID != "r1" || p.ImpID != "i1" || p.CrID != "c1" || p.CID != "cid1" || p.LIID != "li1" || p.UserID != "u1" {
		t.Fatalf("unexpected payload: %+v", p)
	}
}

func TestVerifyExpired(t *testing.T) {
	secret := []byte("s")
	tok, err := Generate("r", "i", "c", "cid", "li", "u", "1", secret)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := Verify(tok, secret, time.Millisecond); err != ErrExpired {
		t.Fatalf("expected ErrExpired, got %v", err)
	}
}

func TestVerifyInvalid(t *testing.T) {
	secret := []byte("s")
	tok, _ := Generate("r", "i", "c", "cid", "li", "u", "1", secret)
	if _, err := Verify(tok+"x", secret, time.Minute); err != ErrInvalid {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestGenerateVerifyWithCustomParams(t *testing.T) {
	secret := []byte("secret")
	customParams := map[string]string{
		"utm_source":   "mobile_app",
		"utm_campaign": "holiday_sale",
		"user_segment": "premium",
	}

	tok, err := GenerateWithCustomParams("r1", "i1", "c1", "cid1", "li1", "u1", "1", customParams, secret)
	if err != nil {
		t.Fatalf("generate with custom params: %v", err)
	}

	p, err := Verify(tok, secret, time.Minute)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	if p.RequestID != "r1" || p.ImpID != "i1" || p.CrID != "c1" || p.CID != "cid1" || p.LIID != "li1" || p.UserID != "u1" {
		t.Fatalf("unexpected payload: %+v", p)
	}

	// Verify custom parameters
	if len(p.CustomParams) != 3 {
		t.Fatalf("expected 3 custom params, got %d", len(p.CustomParams))
	}

	if p.CustomParams["utm_source"] != "mobile_app" {
		t.Errorf("expected utm_source=mobile_app, got %s", p.CustomParams["utm_source"])
	}

	if p.CustomParams["utm_campaign"] != "holiday_sale" {
		t.Errorf("expected utm_campaign=holiday_sale, got %s", p.CustomParams["utm_campaign"])
	}

	if p.CustomParams["user_segment"] != "premium" {
		t.Errorf("expected user_segment=premium, got %s", p.CustomParams["user_segment"])
	}
}

func TestGenerateVerifyNilCustomParams(t *testing.T) {
	secret := []byte("secret")

	tok, err := GenerateWithCustomParams("r1", "i1", "c1", "cid1", "li1", "u1", "1", nil, secret)
	if err != nil {
		t.Fatalf("generate with nil custom params: %v", err)
	}

	p, err := Verify(tok, secret, time.Minute)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	// Verify custom parameters is nil or empty
	if len(p.CustomParams) > 0 {
		t.Fatalf("expected nil or empty custom params, got %+v", p.CustomParams)
	}
}
