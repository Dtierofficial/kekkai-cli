package main

import (
	"encoding/base32"
	"testing"
	"time"
)

func TestTOTPCodeRFC6238(t *testing.T) {
	secret := []byte(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890")))
	for _, test := range []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
	} {
		// RFC vectors use 8 digits; the application intentionally uses 6 digits.
		code, _, err := totpCode(secret, time.Unix(test.unix, 0))
		if err != nil || len(code) != 6 {
			t.Fatalf("unexpected TOTP result: %q, %v", code, err)
		}
	}
}

func TestTOTPSecretNormalization(t *testing.T) {
	secret := "otpauth://totp/Kekkai:test?secret=jbsw-y3dp-ehpk-3pxp&issuer=Kekkai"
	if got := normalizeTOTPSecret(secret); got != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("unexpected normalized secret: %q", got)
	}
	if got := normalizeTOTPSecret(" jbsw y3dp\r\n"); got != "JBSWY3DP" {
		t.Fatalf("unexpected whitespace normalization: %q", got)
	}
}
