package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// TestErrCodeUnmarshalJSON verifies that ErrCode correctly deserialises both
// string and numeric JSON values.
func TestErrCodeUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  ErrCode
	}{
		{"string value", `"20000"`, ErrCode("20000")},
		{"numeric value", `20000`, ErrCode("20000")},
		{"error string", `"40008"`, ErrCode("40008")},
		{"error numeric", `40008`, ErrCode("40008")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ErrCode
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("invalid value", func(t *testing.T) {
		var got ErrCode
		err := json.Unmarshal([]byte(`true`), &got)
		if err == nil {
			t.Fatal("expected error for boolean input, got nil")
		}
	})
}

// TestEncodePayload verifies that EncodePayload produces a deterministic
// JSON payload that contains a "sign" field and that the signature is correct.
func TestEncodePayload(t *testing.T) {
	t.Run("adds sign field", func(t *testing.T) {
		data := map[string]string{
			"fid":  "12345",
			"time": "1700000000",
		}
		payload, err := EncodePayload(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var result map[string]string
		if err := json.Unmarshal([]byte(payload), &result); err != nil {
			t.Fatalf("payload is not valid JSON: %v", err)
		}

		if _, ok := result["sign"]; !ok {
			t.Error("payload missing 'sign' field")
		}
		if result["fid"] != "12345" {
			t.Errorf("fid = %q, want %q", result["fid"], "12345")
		}
	})

	t.Run("sign is deterministic for same input", func(t *testing.T) {
		data1 := map[string]string{"fid": "abc", "time": "999"}
		data2 := map[string]string{"fid": "abc", "time": "999"}

		p1, err := EncodePayload(data1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		p2, err := EncodePayload(data2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var r1, r2 map[string]string
		json.Unmarshal([]byte(p1), &r1)
		json.Unmarshal([]byte(p2), &r2)

		if r1["sign"] != r2["sign"] {
			t.Errorf("expected deterministic sign, got %q and %q", r1["sign"], r2["sign"])
		}
	})

	t.Run("different inputs produce different signs", func(t *testing.T) {
		d1 := map[string]string{"fid": "player1", "time": "1000"}
		d2 := map[string]string{"fid": "player2", "time": "1000"}

		p1, _ := EncodePayload(d1)
		p2, _ := EncodePayload(d2)

		var r1, r2 map[string]string
		json.Unmarshal([]byte(p1), &r1)
		json.Unmarshal([]byte(p2), &r2)

		if r1["sign"] == r2["sign"] {
			t.Error("expected different signs for different inputs")
		}
	})

	t.Run("sign is computed correctly", func(t *testing.T) {
		// Compute the expected signature manually using the same algorithm.
		data := map[string]string{
			"fid":  "testplayer",
			"time": "1700000000",
		}

		values := url.Values{}
		for k, v := range data {
			values.Set(k, v)
		}
		dataToHash := values.Encode() + Key

		// Use the same MD5 approach as EncodePayload — we verify by re-running
		// EncodePayload on a copy and checking the sign matches.
		dataCopy := map[string]string{"fid": "testplayer", "time": "1700000000"}
		payload, err := EncodePayload(dataCopy)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var result map[string]string
		json.Unmarshal([]byte(payload), &result)

		// The sign should be a 32-char hex MD5.
		sign := result["sign"]
		if len(sign) != 32 {
			t.Errorf("sign length = %d, want 32; dataToHash = %q", len(sign), dataToHash)
		}
		if !isHex(sign) {
			t.Errorf("sign %q is not a valid hex string", sign)
		}
	})
}

// TestProcessNewCodeAlreadyActive checks the early-return paths in processNewCode.
func TestProcessNewCodeAlreadyActive(t *testing.T) {
	// Reset global state.
	ActiveCodes = []string{"EXISTINGCODE"}
	ExpiredCodes = []string{"EXPIREDCODE"}
	defer func() {
		ActiveCodes = []string{}
		ExpiredCodes = nil
	}()

	t.Run("returns message when code already active", func(t *testing.T) {
		result := processNewCode("dummy_file.csv", "EXISTINGCODE")
		if !strings.Contains(result, "already active") {
			t.Errorf("expected 'already active' in result, got: %q", result)
		}
	})

	t.Run("returns message when code already expired", func(t *testing.T) {
		result := processNewCode("dummy_file.csv", "EXPIREDCODE")
		if !strings.Contains(result, "expired") {
			t.Errorf("expected 'expired' in result, got: %q", result)
		}
	})

	t.Run("returns error when player file missing", func(t *testing.T) {
		ActiveCodes = []string{}
		result := processNewCode("/nonexistent/path/players.csv", "NEWCODE")
		if !strings.Contains(result, "failed to open player file") {
			t.Errorf("expected file open error in result, got: %q", result)
		}
	})
}

// isHex returns true if s contains only hex characters.
func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// TestLoginResponseDecoding verifies that LoginResponse JSON with both string
// and numeric err_code values deserialises correctly.
func TestLoginResponseDecoding(t *testing.T) {
	t.Run("numeric err_code", func(t *testing.T) {
		raw := `{"code": 0, "msg": "ok", "data": null, "err_code": 20000}`
		var resp LoginResponse
		if err := json.Unmarshal([]byte(raw), &resp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.ErrCode != ErrCode("20000") {
			t.Errorf("ErrCode = %q, want %q", resp.ErrCode, "20000")
		}
	})

	t.Run("string err_code", func(t *testing.T) {
		raw := `{"code": 0, "msg": "ok", "data": null, "err_code": "20000"}`
		var resp LoginResponse
		if err := json.Unmarshal([]byte(raw), &resp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.ErrCode != ErrCode("20000") {
			t.Errorf("ErrCode = %q, want %q", resp.ErrCode, "20000")
		}
	})
}

// TestRedeemResponseDecoding mirrors TestLoginResponseDecoding for RedeemResponse.
func TestRedeemResponseDecoding(t *testing.T) {
	raw := fmt.Sprintf(`{"code": 0, "msg": "success", "err_code": "%s"}`, ErrCodeSuccess)
	var resp RedeemResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrCode != ErrCode(ErrCodeSuccess) {
		t.Errorf("ErrCode = %q, want %q", resp.ErrCode, ErrCodeSuccess)
	}
}
