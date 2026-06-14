package services

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestGetHubReturnsSameInstance(t *testing.T) {
	// Reset the package-level singleton so the test is self-contained.
	hubOnce = sync.Once{}
	hub = nil

	h1 := GetHub()
	h2 := GetHub()
	if h1 != h2 {
		t.Fatal("GetHub() returned different instances")
	}

	// Cleanup: stop the hub so the goroutine doesn't leak.
	h1.Stop()
}

func TestGetHubConcurrentAccess(t *testing.T) {
	hubOnce = sync.Once{}
	hub = nil

	const goroutines = 50
	results := make(chan *Hub, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			results <- GetHub()
		}()
	}
	wg.Wait()
	close(results)

	var first *Hub
	for h := range results {
		if first == nil {
			first = h
		} else if h != first {
			t.Fatal("GetHub() returned different instances under concurrent access")
		}
	}

	first.Stop()
}

func TestHubStopClosesClients(t *testing.T) {
	h := NewHub()
	go h.Run()

	// Register a fake client with a buffered Send channel.
	client := &Client{
		UserID: 1,
		Send:   make(chan []byte, 8),
		hub:    h,
	}
	h.register <- client

	// Give the hub a moment to process the registration.
	time.Sleep(20 * time.Millisecond)

	if h.GetClientCount() != 1 {
		t.Fatalf("expected 1 client, got %d", h.GetClientCount())
	}

	h.Stop()

	// Give the hub a moment to process the stop.
	time.Sleep(20 * time.Millisecond)

	if h.GetClientCount() != 0 {
		t.Fatalf("expected 0 clients after Stop, got %d", h.GetClientCount())
	}

	// The client's Send channel should be closed.
	_, ok := <-client.Send
	if ok {
		t.Fatal("expected client.Send to be closed after hub Stop")
	}
}

func TestHubBroadcastRuntimeAdminFiltersToAdminRuntimeClients(t *testing.T) {
	h := NewHub()
	adminRuntime := &Client{
		UserID: 1,
		Role:   "admin",
		Topic:  WebSocketTopicRuntimeAdmin,
		Send:   make(chan []byte, 1),
		hub:    h,
	}
	adminUserTopic := &Client{
		UserID: 2,
		Role:   "admin",
		Topic:  WebSocketTopicUser,
		Send:   make(chan []byte, 1),
		hub:    h,
	}
	normalRuntime := &Client{
		UserID: 3,
		Role:   "user",
		Topic:  WebSocketTopicRuntimeAdmin,
		Send:   make(chan []byte, 1),
		hub:    h,
	}
	h.clients[adminRuntime] = true
	h.clients[adminUserTopic] = true
	h.clients[normalRuntime] = true

	h.BroadcastRuntimeAdmin("runtime_pod_metrics", map[string]any{"pod_id": int64(9)})

	select {
	case raw := <-adminRuntime.Send:
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("runtime admin message is not json: %v", err)
		}
		if msg.Type != "runtime_pod_metrics" {
			t.Fatalf("message type = %q, want runtime_pod_metrics", msg.Type)
		}
	default:
		t.Fatalf("admin runtime client did not receive runtime admin message")
	}

	select {
	case raw := <-adminUserTopic.Send:
		t.Fatalf("admin user-topic client received runtime admin message: %s", string(raw))
	default:
	}

	select {
	case raw := <-normalRuntime.Send:
		t.Fatalf("normal runtime client received runtime admin message: %s", string(raw))
	default:
	}
}
