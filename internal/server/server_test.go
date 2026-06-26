package server_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/get-robotunnel/roboar/internal/auth"
	"github.com/get-robotunnel/roboar/internal/config"
	"github.com/get-robotunnel/roboar/internal/server"
	"github.com/get-robotunnel/roboar/internal/store"
	"github.com/gin-gonic/gin"
)

// newTestServer builds a server against TEST_DATABASE_URL and resets all tables.
// The test is skipped when no test database is configured.
func newTestServer(t *testing.T) (*gin.Engine, *store.Store) {
	t.Helper()
	dburl := os.Getenv("TEST_DATABASE_URL")
	if dburl == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping API integration test")
	}
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	st, err := store.New(ctx, dburl)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := st.Pool.Exec(ctx,
		`TRUNCATE owners, platforms, agents, capabilities, platform_configs RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	cfg := &config.Config{Port: "0", DatabaseURL: dburl, JWTSigningKey: "testsecret",
		BaseURL: "http://test/v1", OfflineAfterSecs: 60}
	srv := server.New(cfg, st, auth.NewManager(cfg.JWTSigningKey))
	t.Cleanup(st.Close)
	return srv.Engine(), st
}

type req struct {
	method, path, authScheme, authToken string
	body                                interface{}
}

func do(t *testing.T, e *gin.Engine, r req) (int, map[string]interface{}) {
	t.Helper()
	var buf bytes.Buffer
	if r.body != nil {
		_ = json.NewEncoder(&buf).Encode(r.body)
	}
	httpReq := httptest.NewRequest(r.method, r.path, &buf)
	if r.authToken != "" {
		httpReq.Header.Set("Authorization", r.authScheme+" "+r.authToken)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httpReq)

	var out map[string]interface{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func TestEndToEndDiscovery(t *testing.T) {
	e, st := newTestServer(t)
	ctx := context.Background()

	// 1. Owner registers with an Ed25519 public key.
	pub, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(pub)
	code, body := do(t, e, req{method: "POST", path: "/v1/owners",
		body: map[string]string{"public_key": pubHex, "display_name": "Russell"}})
	if code != http.StatusCreated {
		t.Fatalf("create owner: code=%d body=%v", code, body)
	}

	// 2. Owner logs in via challenge-response → JWT.
	_, ch := do(t, e, req{method: "POST", path: "/v1/owners/auth/challenge",
		body: map[string]string{"public_key": pubHex}})
	challenge := ch["challenge"].(string)
	sig := hex.EncodeToString(ed25519.Sign(priv, []byte(challenge)))
	code, ver := do(t, e, req{method: "POST", path: "/v1/owners/auth/verify",
		body: map[string]string{"public_key": pubHex, "challenge": challenge, "signature": sig}})
	if code != http.StatusOK {
		t.Fatalf("verify: code=%d body=%v", code, ver)
	}
	jwt := ver["token"].(string)

	// 3. Register a platform → platform_token (returned once).
	code, plt := do(t, e, req{method: "POST", path: "/v1/platforms", authScheme: "Bearer", authToken: jwt,
		body: map[string]interface{}{"platform_type": "raspberry_pi", "display_name": "RPi 4", "tags": []string{"lidar", "outdoor"}}})
	if code != http.StatusCreated {
		t.Fatalf("create platform: code=%d body=%v", code, plt)
	}
	platformToken := plt["platform_token"].(string)
	platformID := plt["platform"].(map[string]interface{})["platform_id"].(string)

	// 4. Register an agent with the platform token.
	code, agt := do(t, e, req{method: "POST", path: "/v1/platforms/" + platformID + "/agents",
		authScheme: "Platform", authToken: platformToken,
		body: map[string]interface{}{"name": "operations-agent", "description": "Manages this robot",
			"agent_type": "operations", "version": "1.0.0", "visibility": "public"}})
	if code != http.StatusCreated {
		t.Fatalf("create agent: code=%d body=%v", code, agt)
	}
	agentID := agt["agent_id"].(string)

	// 5. Register a public capability.
	code, _ = do(t, e, req{method: "POST", path: "/v1/agents/" + agentID + "/capabilities",
		authScheme: "Platform", authToken: platformToken,
		body: map[string]interface{}{"name": "get_system_status", "display_name": "Get System Status",
			"interface_type": "mcp_tool", "permission": "public"}})
	if code != http.StatusCreated {
		t.Fatalf("create capability: code=%d", code)
	}

	// 6. Heartbeat → platform/agent become online.
	code, _ = do(t, e, req{method: "POST", path: "/v1/platforms/" + platformID + "/heartbeat",
		authScheme: "Platform", authToken: platformToken, body: map[string]string{"status": "online"}})
	if code != http.StatusOK {
		t.Fatalf("heartbeat: code=%d", code)
	}

	// 7. Anonymous discovery finds the online agent with its capability.
	code, disc := do(t, e, req{method: "GET", path: "/v1/discover/agents?online=true"})
	if code != http.StatusOK {
		t.Fatalf("discover: code=%d", code)
	}
	if total := disc["total"].(float64); total != 1 {
		t.Fatalf("expected 1 agent online, got %v", total)
	}
	agents := disc["agents"].([]interface{})
	first := agents[0].(map[string]interface{})
	if first["online"].(bool) != true {
		t.Fatal("expected agent online=true")
	}
	if caps := first["capabilities"].([]interface{}); len(caps) != 1 {
		t.Fatalf("expected 1 capability, got %d", len(caps))
	}

	// 8. Simulate the agent going stale → it drops out of online discovery.
	if _, err := st.Pool.Exec(ctx,
		`UPDATE platforms SET last_seen_at = NOW() - INTERVAL '120 seconds' WHERE platform_id=$1`, platformID); err != nil {
		t.Fatalf("age platform: %v", err)
	}
	code, disc = do(t, e, req{method: "GET", path: "/v1/discover/agents?online=true"})
	if code != http.StatusOK {
		t.Fatalf("discover after offline: code=%d", code)
	}
	if total := disc["total"].(float64); total != 0 {
		t.Fatalf("expected 0 online agents after staleness, got %v", total)
	}

	// 9. Direct discovery-by-id still works (offline) and reports online=false.
	code, one := do(t, e, req{method: "GET", path: "/v1/discover/agents/" + agentID})
	if code != http.StatusOK {
		t.Fatalf("discover by id: code=%d", code)
	}
	if one["online"].(bool) != false {
		t.Fatal("expected online=false after staleness")
	}
}
