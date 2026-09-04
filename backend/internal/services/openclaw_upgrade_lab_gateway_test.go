package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestUpgradeLabConversationClientCompletesHandshakeAndChat(t *testing.T) {
	upgrader := websocket.Upgrader{}
	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-Prefix") != "/api/v1/instances/77/proxy" || r.Header.Get("X-Forwarded-Proto") != "https" {
			t.Errorf("trusted proxy identity headers are missing")
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_ = conn.WriteJSON(map[string]any{"type": "event", "event": "connect.challenge", "payload": map[string]any{"nonce": "nonce-1"}})
		for i := 0; i < 2; i++ {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				t.Error(err)
				return
			}
			var req struct {
				ID     string         `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if json.Unmarshal(raw, &req) != nil {
				t.Errorf("invalid request: %s", raw)
				return
			}
			received <- req.Method
			if req.Method == "connect" {
				auth, _ := req.Params["auth"].(map[string]any)
				if auth["token"] != "test-token" {
					t.Errorf("gateway token was not sent")
				}
			}
			_ = conn.WriteJSON(map[string]any{"type": "res", "id": req.ID, "ok": true, "payload": map[string]any{"status": "ok"}})
		}
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(parsed.Port())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := openClawUpgradeLabConversationClient{}
	if err := client.SendMessage(ctx, "http://"+parsed.Hostname()+":19090", port, 77, "test-token", "agent:main:test", "hello"); err != nil {
		t.Fatal(err)
	}
	if first, second := <-received, <-received; first != "connect" || second != "chat.send" {
		t.Fatalf("methods = %s, %s", first, second)
	}
}

func TestUpgradeLabConversationClientRejectsHandshakeFailureWithoutLeakingMessage(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()
		_ = conn.WriteJSON(map[string]any{"type": "event", "event": "connect.challenge", "payload": map[string]any{"nonce": "nonce-1"}})
		_, raw, _ := conn.ReadMessage()
		var req struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &req)
		_ = conn.WriteJSON(map[string]any{"type": "res", "id": req.ID, "ok": false, "error": map[string]any{"code": "AUTH_FAILED", "message": "secret detail"}})
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(parsed.Port())
	err := (openClawUpgradeLabConversationClient{}).SendMessage(context.Background(), "http://"+parsed.Hostname()+":19090", port, 77, "bad-token", "agent:main:test", "sensitive message")
	if err == nil || err.Error() != "baseline gateway handshake: request rejected (AUTH_FAILED)" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpgradeLabConversationClientAgainstConfiguredGateway(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("OPENCLAW_LAB_GATEWAY_ENDPOINT"))
	token := strings.TrimSpace(os.Getenv("OPENCLAW_LAB_GATEWAY_TOKEN"))
	if endpoint == "" || token == "" {
		t.Skip("real gateway endpoint is not configured")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := (openClawUpgradeLabConversationClient{}).SendMessage(ctx, "http://"+parsed.Hostname()+":19090", port, 77, token, "agent:main:upgrade-lab-protocol-test", "请回复收到"); err != nil {
		t.Fatal(err)
	}
}
