package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SignRequest computes an HMAC-SHA256 signature for an API request.
// The signature covers method, path, body, and timestamp to prevent replay.
func SignRequest(secret, method, path, body, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strings.Join([]string{method, path, body, timestamp}, ":")))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks whether the given signature matches the expected HMAC
// computed from the request parameters.
func VerifySignature(secret, method, path, body, timestamp, signature string) bool {
	expected := SignRequest(secret, method, path, body, timestamp)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// ValidateTimestamp checks that the provided UNIX-epoch timestamp string
// falls within the allowed time window (now ± maxAge).
func ValidateTimestamp(timestampStr string, maxAge time.Duration) error {
	if timestampStr == "" {
		return fmt.Errorf("missing timestamp")
	}

	ts, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp format: %w", err)
	}

	requestTime := time.Unix(ts, 0)
	now := time.Now()
	diff := now.Sub(requestTime)
	if diff < 0 {
		diff = -diff
	}

	if diff > maxAge {
		allowedSec := int(maxAge.Seconds())
		if allowedSec >= 60 {
			return fmt.Errorf("timestamp expired (allowed ±%dm)", allowedSec/60)
		}
		return fmt.Errorf("timestamp expired (allowed ±%ds)", allowedSec)
	}

	return nil
}
