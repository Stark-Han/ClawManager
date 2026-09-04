package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type upgradeLabConversationClient interface {
	SendMessage(context.Context, string, int, int, string, string, string) error
}

type openClawUpgradeLabConversationClient struct{}

type openClawGatewayFrame struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	OK      bool            `json:"ok,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
	Event   string          `json:"event,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func (openClawUpgradeLabConversationClient) SendMessage(ctx context.Context, agentEndpoint string, gatewayPort, instanceID int, gatewayToken, sessionKey, message string) error {
	endpoint, err := upgradeLabGatewayWebSocketURL(agentEndpoint, gatewayPort)
	if err != nil {
		return err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: nil}
	headers := http.Header{}
	proxyIdentity := fmt.Sprintf("/api/v1/instances/%d/proxy", instanceID)
	headers.Set("Authorization", "Bearer "+gatewayToken)
	headers.Set("X-Api-Key", gatewayToken)
	headers.Set("X-OpenAI-Api-Key", gatewayToken)
	headers.Set("OpenAI-Api-Key", gatewayToken)
	headers.Set("X-ClawManager-Instance-Token", gatewayToken)
	headers.Set("X-ClawManager-LLM-API-Key", gatewayToken)
	headers.Set("X-Forwarded-For", "192.0.2.1")
	headers.Set("X-Forwarded-Host", "clawmanager-upgrade-lab")
	headers.Set("X-Forwarded-Proto", "https")
	headers.Set("X-Forwarded-Prefix", proxyIdentity)
	headers.Set("Origin", strings.Replace(endpoint, "ws://", "http://", 1))
	conn, response, err := dialer.DialContext(ctx, endpoint, headers)
	if err != nil {
		if response != nil {
			return fmt.Errorf("connect to baseline gateway: HTTP %d", response.StatusCode)
		}
		return fmt.Errorf("connect to baseline gateway: %w", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := waitUpgradeLabGatewayChallenge(conn); err != nil {
		return fmt.Errorf("baseline gateway challenge: %w", err)
	}
	connectID := upgradeLabRequestID()
	connect := map[string]any{
		"type": "req", "id": connectID, "method": "connect",
		"params": map[string]any{
			"minProtocol": 4, "maxProtocol": 4,
			"client": map[string]any{"id": "openclaw-control-ui", "version": "clawmanager-upgrade-lab", "platform": "linux", "mode": "ui"},
			"role":   "operator", "scopes": []string{"operator.admin", "operator.read", "operator.write"},
			"auth": map[string]any{"token": gatewayToken},
		},
	}
	if err := conn.WriteJSON(connect); err != nil {
		return fmt.Errorf("send baseline gateway handshake: %w", err)
	}
	if err := waitUpgradeLabGatewayResponse(conn, connectID); err != nil {
		return fmt.Errorf("baseline gateway handshake: %w", err)
	}
	requestID := upgradeLabRequestID()
	request := map[string]any{
		"type": "req", "id": requestID, "method": "chat.send",
		"params": map[string]any{
			"sessionKey": sessionKey, "message": message, "deliver": false,
			"idempotencyKey": upgradeLabRequestID(),
		},
	}
	if err := conn.WriteJSON(request); err != nil {
		return fmt.Errorf("send baseline conversation: %w", err)
	}
	if err := waitUpgradeLabGatewayResponse(conn, requestID); err != nil {
		return fmt.Errorf("baseline conversation: %w", err)
	}
	return nil
}

func waitUpgradeLabGatewayChallenge(conn *websocket.Conn) error {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var frame openClawGatewayFrame
		if json.Unmarshal(raw, &frame) != nil || frame.Type != "event" || frame.Event != "connect.challenge" {
			continue
		}
		var payload struct {
			Nonce string `json:"nonce"`
		}
		if json.Unmarshal(frame.Payload, &payload) != nil || strings.TrimSpace(payload.Nonce) == "" {
			return errors.New("challenge nonce is missing")
		}
		return nil
	}
}

func upgradeLabGatewayWebSocketURL(agentEndpoint string, gatewayPort int) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(agentEndpoint))
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("upgrade lab Runtime agent endpoint is invalid")
	}
	if gatewayPort <= 0 || gatewayPort > 65535 {
		return "", errors.New("upgrade lab gateway port is invalid")
	}
	// Gateway ports are assigned independently by the caller; the agent URL is
	// used only to obtain the pod address.
	return "ws://" + net.JoinHostPort(parsed.Hostname(), strconv.Itoa(gatewayPort)), nil
}

func waitUpgradeLabGatewayResponse(conn *websocket.Conn, requestID string) error {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var frame openClawGatewayFrame
		if json.Unmarshal(raw, &frame) != nil || frame.Type != "res" || frame.ID != requestID {
			continue
		}
		if frame.OK {
			return nil
		}
		return fmt.Errorf("request rejected (%s)", safeUpgradeLabGatewayError(frame.Error))
	}
}

func safeUpgradeLabGatewayError(raw json.RawMessage) string {
	var value struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &value) == nil {
		if strings.TrimSpace(value.Code) != "" {
			return strings.TrimSpace(value.Code)
		}
		if strings.TrimSpace(value.Message) != "" {
			return "gateway_error"
		}
	}
	return "gateway_error"
}

func upgradeLabRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(raw[:])
}
