package checks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxSignatureSkewSeconds = 60

func getSignatureSecret() string {
	for _, key := range []string{"INTERNAL_API_SECRET", "API_SIGNATURE_SECRET"} {
		if val := strings.TrimSpace(os.Getenv(key)); val != "" {
			return val
		}
	}
	return ""
}

func computeRequestSignature(secret, timestamp, method, path string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write([]byte(strings.ToUpper(method)))
	mac.Write([]byte("."))
	mac.Write([]byte(path))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func verifySignedRequest(r *http.Request) error {
	secret := getSignatureSecret()
	if secret == "" {
		return fmt.Errorf("signature secret is not configured")
	}

	tsHeader := strings.TrimSpace(r.Header.Get("X-Timestamp"))
	sigHeader := strings.TrimSpace(r.Header.Get("X-Signature"))
	if tsHeader == "" || sigHeader == "" {
		return fmt.Errorf("missing signature headers")
	}

	ts, err := strconv.ParseInt(tsHeader, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}

	now := time.Now().UTC().Unix()
	if math.Abs(float64(now-ts)) > maxSignatureSkewSeconds {
		return fmt.Errorf("timestamp expired")
	}

	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			return fmt.Errorf("failed to read body")
		}
		// Возвращаем тело обратно, чтобы хендлер мог его декодировать
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	expectedHex := computeRequestSignature(secret, tsHeader, r.Method, r.URL.Path, bodyBytes)
	expectedBytes, err1 := hex.DecodeString(expectedHex)
	providedBytes, err2 := hex.DecodeString(sigHeader)
	if err1 != nil || err2 != nil || !hmac.Equal(expectedBytes, providedBytes) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// RequireSignature оборачивает конкретный http.HandlerFunc
func RequireSignature(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := verifySignedRequest(r); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "INVALID_SIGNATURE", err.Error())
			return
		}
		next(w, r)
	}
}

func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"ok":false,"code":%q,"error":%q}`, code, msg)
}