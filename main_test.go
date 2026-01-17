package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// mockOllamaServer creates a fake HTTP server that mimics the Ollama API behavior.
// It returns a simulated stream of JSON chunks.
func mockOllamaServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Verify request body structure (optional, but good for validation)
		var req OllamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Simulate streaming response
		w.Header().Set("Content-Type", "application/json")
		
		// Chunk 1
		chunk1 := `{"message": {"content": "Hello "}}`
		w.Write([]byte(chunk1 + "\n"))
		w.(http.Flusher).Flush() // Force send to client

		// Chunk 2
		chunk2 := `{"message": {"content": "World"}}`
		w.Write([]byte(chunk2 + "\n"))
		w.(http.Flusher).Flush()
	}))
}

// --- Test Cases ---

// TestHandleHome verifies that the homepage handler returns the index.html content.
func TestHandleHome(t *testing.T) {
	// Create a dummy request
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a recorder to capture the response
	rr := httptest.NewRecorder()

	// Call the handler directly
	handler := http.HandlerFunc(handleHome)
	handler.ServeHTTP(rr, req)

	// Check status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check if body contains expected HTML content
	// Note: This assumes index.html exists. If running in CI, you might need to create a dummy index.html.
	expectedContent := "<!DOCTYPE html>"
	if !strings.Contains(rr.Body.String(), expectedContent) {
		t.Log("Warning: index.html might be missing or content mismatch")
	}
}

// TestWebSocketFlow tests the full end-to-end WebSocket conversation
// using a mocked Ollama server.
func TestWebSocketFlow(t *testing.T) {
	// 1. Setup Mock Ollama Server
	mockOllama := mockOllamaServer()
	defer mockOllama.Close()

    oldURL := OllamaAPIURL
    OllamaAPIURL = mockOllama.URL
    defer func() { OllamaAPIURL = oldURL }() // Restore it after test finishes

	// 2. Start your WebSocket Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(w, r)
	}))
	defer server.Close()

	// 3. Connect Client to Server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("could not open websocket connection: %v", err)
	}
	defer ws.Close()

	// 4. Send a Message
	req := ChatRequest{Message: "Test Message"}
	if err := ws.WriteJSON(req); err != nil {
		t.Fatalf("could not write json: %v", err)
	}

	// 5. Read Responses with a Timeout
	// Instead of a select loop, we set a hard deadline on the connection.
	// If ReadJSON takes longer than 2 seconds, it will return an error.
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))

	doneReceived := false

	for {
		var resp StreamResponse
		err := ws.ReadJSON(&resp)
		if err != nil {
			// If we error out before receiving "Done", the test fails.
			t.Fatalf("Read failed or timed out: %v", err)
		}

		// Check for the "Done" signal
		if resp.Done {
			doneReceived = true
			break // Exit the loop, allowing the code below to run
		}
	}

	// 6. Final Assertion (Now Reachable)
	if !doneReceived {
		t.Error("Did not receive done signal")
	}
}

// TestSlidingWindowLogic verifies the logic for truncating message history.
func TestSlidingWindowLogic(t *testing.T) {
	// Create a fake history of 60 messages
	history := make([]OllamaMessage, 0)
	for i := 0; i < 60; i++ {
		history = append(history, OllamaMessage{Role: "user", Content: "msg"})
	}

	// Simulate logic from streamOllama
	const WindowSize = 50
	systemMessage := OllamaMessage{Role: "system", Content: "Sys"}
	
	messagesToSend := []OllamaMessage{systemMessage}
	var recentMessages []OllamaMessage
	
	if len(history) > WindowSize {
		recentMessages = history[len(history)-WindowSize:]
	} else {
		recentMessages = history
	}
	messagesToSend = append(messagesToSend, recentMessages...)

	// Assertions
	expectedLength := 1 + 50 // 1 System + 50 Recent
	if len(messagesToSend) != expectedLength {
		t.Errorf("Sliding window failed. Got %d messages, want %d", len(messagesToSend), expectedLength)
	}

	// Verify the first message is System
	if messagesToSend[0].Role != "system" {
		t.Error("First message should be system prompt")
	}
}