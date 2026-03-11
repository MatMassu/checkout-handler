package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidSignature = errors.New("invalid webhook signature")

func parsePaymentID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("payment id %q is not a valid integer: %w", s, err)
	}
	return id, nil
}

// validateSignature verifies the x-signature header against the webhook secret.
//
// MercadoPago signs notifications using the template:
//
//	id:{data.id};request-id:{x-request-id};ts:{ts};
//
// Fields are omitted from the template if they are empty.
func validateSignature(xSignature, xRequestID, dataID, secret string) error {
	var ts, v1 string
	for _, part := range strings.Split(xSignature, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "ts="):
			ts = strings.TrimPrefix(part, "ts=")
		case strings.HasPrefix(part, "v1="):
			v1 = strings.TrimPrefix(part, "v1=")
		}
	}

	if ts == "" || v1 == "" {
		return fmt.Errorf("%w: missing ts or v1 in x-signature header", ErrInvalidSignature)
	}

	var sb strings.Builder
	if dataID != "" {
		fmt.Fprintf(&sb, "id:%s;", dataID)
	}
	if xRequestID != "" {
		fmt.Fprintf(&sb, "request-id:%s;", xRequestID)
	}
	fmt.Fprintf(&sb, "ts:%s;", ts)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sb.String()))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(v1)) {
		return ErrInvalidSignature
	}
	return nil
}
