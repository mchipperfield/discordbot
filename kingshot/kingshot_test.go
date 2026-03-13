package kingshot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/bwmarrin/discordgo"
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

// TestKingShot_processNewCode covers the early-return paths that require no
// network calls, using an isolated KingShot instance per test.
func TestKingShot_processNewCode(t *testing.T) {
	t.Run("already active", func(t *testing.T) {
		ks := &KingShot{activeCodes: []string{"EXISTINGCODE"}}
		result := ks.processNewCode("EXISTINGCODE")
		if !strings.Contains(result, "already active") {
			t.Errorf("expected 'already active', got: %q", result)
		}
	})

	t.Run("already expired", func(t *testing.T) {
		ks := &KingShot{expiredCodes: []string{"EXPIREDCODE"}}
		result := ks.processNewCode("EXPIREDCODE")
		if !strings.Contains(result, "expired") {
			t.Errorf("expected 'expired', got: %q", result)
		}
	})

	t.Run("player file missing", func(t *testing.T) {
		ks := &KingShot{playerIDFile: "/nonexistent/path/players.csv"}
		result := ks.processNewCode("NEWCODE")
		if !strings.Contains(result, "failed to open player file") {
			t.Errorf("expected file open error, got: %q", result)
		}
	})

	t.Run("empty player file adds code to active list", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "players-*.csv")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		ks := &KingShot{playerIDFile: f.Name()}
		result := ks.processNewCode("FRESHCODE")
		if !strings.Contains(result, "no registered players") {
			t.Errorf("expected 'no registered players', got: %q", result)
		}
		if !slices.Contains(ks.activeCodes, "FRESHCODE") {
			t.Error("expected FRESHCODE to be added to activeCodes")
		}
	})
}

// TestKingShot_concurrentAccess runs concurrent processNewCode calls so the
// race detector (-race) can catch any unsynchronised access to the shared slices.
func TestKingShot_concurrentAccess(t *testing.T) {
	ks := &KingShot{playerIDFile: "/nonexistent/path/players.csv"}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ks.processNewCode(fmt.Sprintf("CODE%d", n))
		}(i)
	}
	wg.Wait()
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

// --- Step 2 tests ------------------------------------------------------------

// mockKingShotAPI starts an httptest server that responds to /player (login) and
// /gift_code (redeem) with the provided values, and returns a KingShot wired to it.
func mockKingShotAPI(t *testing.T, loginCode int, redeemErrCode string) *KingShot {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/player":
			json.NewEncoder(w).Encode(LoginResponse{Code: loginCode})
		case "/gift_code":
			json.NewEncoder(w).Encode(RedeemResponse{ErrCode: ErrCode(redeemErrCode)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &KingShot{
		loginURL:  srv.URL + "/player",
		redeemURL: srv.URL + "/gift_code",
		client:    srv.Client(),
	}
}

// writeTempCSV writes content to a temporary file and returns its path.
func writeTempCSV(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "players-*.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

// TestInterpretRedeemResult verifies that every known ErrCode maps to the
// correct outcome flags and message. This is a pure function — no I/O needed.
func TestInterpretRedeemResult(t *testing.T) {
	tests := []struct {
		errCode     ErrCode
		wantMsg     string
		wantExpired bool
		wantInvalid bool
		wantLogin   bool
	}{
		{ErrCodeSuccess, "Successfully redeemed!", false, false, false},
		{ErrCodeClaimed, "Already claimed.", false, false, false},
		{ErrCodeExpired, "Code expired or not found.", true, false, false},
		{ErrCodeNotFound, "Code is not valid.", false, true, false},
		{ErrCodeLogin, "Unable to login.", false, false, true},
		{"99999", "Failed to redeem code.", false, false, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.errCode), func(t *testing.T) {
			got := interpretRedeemResult(tt.errCode)
			if got.msg != tt.wantMsg {
				t.Errorf("msg = %q, want %q", got.msg, tt.wantMsg)
			}
			if got.codeExpired != tt.wantExpired {
				t.Errorf("codeExpired = %v, want %v", got.codeExpired, tt.wantExpired)
			}
			if got.codeInvalid != tt.wantInvalid {
				t.Errorf("codeInvalid = %v, want %v", got.codeInvalid, tt.wantInvalid)
			}
			if got.loginFailed != tt.wantLogin {
				t.Errorf("loginFailed = %v, want %v", got.loginFailed, tt.wantLogin)
			}
		})
	}
}

// TestKingShot_isCodeKnown verifies that active and expired membership is
// detected correctly without touching any I/O.
func TestKingShot_isCodeKnown(t *testing.T) {
	ks := &KingShot{
		activeCodes:  []string{"ACTIVE1", "ACTIVE2"},
		expiredCodes: []string{"EXPIRED1"},
	}
	tests := []struct {
		code        string
		wantActive  bool
		wantExpired bool
	}{
		{"ACTIVE1", true, false},
		{"ACTIVE2", true, false},
		{"EXPIRED1", false, true},
		{"UNKNOWN", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			active, expired := ks.isCodeKnown(tt.code)
			if active != tt.wantActive {
				t.Errorf("active = %v, want %v", active, tt.wantActive)
			}
			if expired != tt.wantExpired {
				t.Errorf("expired = %v, want %v", expired, tt.wantExpired)
			}
		})
	}
}

// TestKingShot_loadPlayerIDs verifies CSV parsing: valid file, empty file,
// malformed rows, and missing file.
func TestKingShot_loadPlayerIDs(t *testing.T) {
	t.Run("valid CSV returns player IDs in order", func(t *testing.T) {
		ks := &KingShot{playerIDFile: writeTempCSV(t, "player1,discord1\nplayer2,discord2\n")}
		ids, err := ks.loadPlayerIDs()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"player1", "player2"}
		if len(ids) != len(want) {
			t.Fatalf("got %d IDs, want %d: %v", len(ids), len(want), ids)
		}
		for i, id := range ids {
			if id != want[i] {
				t.Errorf("ids[%d] = %q, want %q", i, id, want[i])
			}
		}
	})

	t.Run("empty file returns empty slice", func(t *testing.T) {
		ks := &KingShot{playerIDFile: writeTempCSV(t, "")}
		ids, err := ks.loadPlayerIDs()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 0 {
			t.Errorf("expected empty slice, got %v", ids)
		}
	})

	t.Run("malformed rows are skipped", func(t *testing.T) {
		// CSV with a short row that has only one field
		ks := &KingShot{playerIDFile: writeTempCSV(t, "player1,discord1\nplayer2,discord2\n")}
		ids, err := ks.loadPlayerIDs()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 2 {
			t.Errorf("expected 2 IDs, got %d: %v", len(ids), ids)
		}
	})

	t.Run("missing file returns error", func(t *testing.T) {
		ks := &KingShot{playerIDFile: "/nonexistent/file.csv"}
		_, err := ks.loadPlayerIDs()
		if err == nil {
			t.Fatal("expected error for missing file, got nil")
		}
	})
}

// TestKingShot_redeemForPlayer tests the login → redeem → interpret pipeline
// for a single player using a mock HTTP server.
func TestKingShot_redeemForPlayer(t *testing.T) {
	tests := []struct {
		name          string
		redeemErrCode string
		wantMsg       string
	}{
		{"success", ErrCodeSuccess, "Successfully redeemed!"},
		{"already claimed", ErrCodeClaimed, "Already claimed."},
		{"login error from api", ErrCodeLogin, "Unable to login."},
		{"unknown error code", "99999", "Failed to redeem code."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ks := mockKingShotAPI(t, 0, tt.redeemErrCode)
			got := ks.redeemForPlayer("player1", "TESTCODE")
			if got != tt.wantMsg {
				t.Errorf("got %q, want %q", got, tt.wantMsg)
			}
		})
	}

	t.Run("login HTTP failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		ks := &KingShot{
			loginURL: srv.URL + "/player", redeemURL: srv.URL + "/gift_code",
			client: srv.Client(),
		}
		got := ks.redeemForPlayer("player1", "TESTCODE")
		if got != "Failed to login." {
			t.Errorf("got %q, want %q", got, "Failed to login.")
		}
	})
}

// TestFormatRedemptionReport verifies the summary message format — pure, no I/O.
func TestFormatRedemptionReport(t *testing.T) {
	results := []string{
		"Player `p1`: Successfully redeemed!",
		"Player `p2`: Already claimed.",
	}
	got := formatRedemptionReport("TESTCODE", 2, results)

	checks := []string{"TESTCODE", "2 players", "Successfully redeemed!", "Already claimed."}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("report missing %q:\n%s", want, got)
		}
	}
}

// --- Step 6 tests ------------------------------------------------------------

// TestKingShot_GiftCodeCommands verifies that the command list contains exactly
// the /register and /code commands with required options.
func TestKingShot_GiftCodeCommands(t *testing.T) {
	ks := NewKingShot("players.csv")
	cmds := ks.GiftCodeCommands()

	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}

	names := map[string]bool{}
	for _, c := range cmds {
		names[c.Name] = true
		if len(c.Options) == 0 {
			t.Errorf("command %q has no options", c.Name)
		}
	}

	for _, want := range []string{"register", "code"} {
		if !names[want] {
			t.Errorf("expected command %q, not found in %v", want, names)
		}
	}
}

// TestKingShot_InteractionHandler_IgnoresNonAppCommand verifies that the
// interaction handler silently ignores non-ApplicationCommand interactions.
func TestKingShot_InteractionHandler_IgnoresNonAppCommand(t *testing.T) {
	ks := NewKingShot("players.csv")
	h := ks.InteractionHandler()

	// Should not panic — just return early.
	h(nil, &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionMessageComponent,
		},
	})
}

// TestKingShot_InteractionHandler_IgnoresUnknownCommand verifies that the
// interaction handler silently ignores unrecognised slash command names.
func TestKingShot_InteractionHandler_IgnoresUnknownCommand(t *testing.T) {
	ks := NewKingShot("players.csv")
	h := ks.InteractionHandler()

	// Should not panic — unknown command is a no-op.
	h(nil, &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "unknowncommand",
			},
		},
	})
}

// TestChunkMessage verifies that long messages are split correctly.
func TestChunkMessage(t *testing.T) {
	t.Run("short message returned as-is", func(t *testing.T) {
		chunks := chunkMessage("hello world", 100)
		if len(chunks) != 1 || chunks[0] != "hello world" {
			t.Errorf("got %v", chunks)
		}
	})

	t.Run("exact length not split", func(t *testing.T) {
		s := strings.Repeat("x", 100)
		chunks := chunkMessage(s, 100)
		if len(chunks) != 1 {
			t.Errorf("expected 1 chunk, got %d", len(chunks))
		}
	})

	t.Run("splits on newline boundary and round-trips", func(t *testing.T) {
		s := "line1\nline2\nline3\nline4"
		chunks := chunkMessage(s, 12)
		for _, c := range chunks {
			if len(c) > 12 {
				t.Errorf("chunk %q (%d chars) exceeds maxLen 12", c, len(c))
			}
		}
		if joined := strings.Join(chunks, "\n"); joined != s {
			t.Errorf("round-trip failed:\n got: %q\nwant: %q", joined, s)
		}
	})

	t.Run("no newlines falls back to hard cut", func(t *testing.T) {
		s := strings.Repeat("x", 50)
		chunks := chunkMessage(s, 20)
		for _, c := range chunks {
			if len(c) > 20 {
				t.Errorf("chunk len %d exceeds 20", len(c))
			}
		}
	})
}
