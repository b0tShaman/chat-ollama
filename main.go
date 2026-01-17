package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/gorilla/websocket"
	"golang.ngrok.com/ngrok"
	"golang.ngrok.com/ngrok/config"
)

var OllamaAPIURL = "http://localhost:11434/api/chat"

// Configure the Upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all connections
	},
}

// Structs
type ChatRequest struct {
	Message string `json:"message"`
}

type StreamResponse struct {
	Chunk string `json:"chunk"`
	Done  bool   `json:"done"`
}

type OllamaRequest struct {
	Model    string                 `json:"model"`
	Messages []OllamaMessage        `json:"messages"`
	Stream   bool                   `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func main() {
	checkOllama()

	// 1. Setup Handlers (Once globally)
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/ws", handleWebSocket)

	// 2. Parse Mode (Default to 'local')
	mode := "local"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	// 3. Start Server based on mode
	switch mode {
	case "ngrok":
		log.Println("🌍 Exposing server via ngrok...")
		ExposeViaNgrok() // This blocks execution
	case "lan":
		ip, err := GetLocalIP()
		if err != nil {
			ip = "0.0.0.0"
		}
		port := ":8080"
		log.Printf("🤖 LAN Server running at http://%s%s\n", ip, port)
		// Listen on all interfaces
		if err := http.ListenAndServe("0.0.0.0"+port, nil); err != nil {
			log.Fatal(err)
		}
	default: // "local"
		port := ":8080"
		log.Printf("🤖 Local Server running at http://localhost%s\n", port)
		// Listen strictly on localhost
		if err := http.ListenAndServe("localhost"+port, nil); err != nil {
			log.Fatal(err)
		}
	}
}

func checkOllama() {
	_, err := exec.LookPath("ollama")
	if err != nil {
		log.Println("⚠️  Warning: Ollama is not installed or not in your PATH.")
		switch runtime.GOOS {
		case "windows":
			log.Println("👉 Download: https://ollama.com/download/windows")
		case "darwin":
			log.Println("👉 Download: https://ollama.com/download/mac")
		default:
			log.Println("👉 Run: curl -fsSL https://ollama.com/install.sh | sh")
		}
	} else {
		log.Println("✅ Ollama found.")
	}
}

func ExposeViaNgrok() {
	if err := runNgrok(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func runNgrok(ctx context.Context) error {
	log.Println("[Debug] Getting Authtoken from environment...")

	// Check if token exists
	token := os.Getenv("NGROK_AUTHTOKEN")
	if token == "" {
		return fmt.Errorf("❌ ERROR: NGROK_AUTHTOKEN is empty. Please export it before running")
	}
	log.Println("[Debug] Token found. Attempting to connect to Ngrok Cloud...")

	// Attempt connection
	listener, err := ngrok.Listen(ctx,
		config.HTTPEndpoint(),
		ngrok.WithAuthtokenFromEnv(),
	)
	if err != nil {
		log.Printf("[Debug] ❌ Connection Failed: %v\n", err)
		return err
	}

	// Success
	log.Println("Debug] ✅ Connection Success!")
	log.Println("Ingress established at:", listener.URL())

	// Serve
	return http.Serve(listener, nil)
}

func GetLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// --- Handlers ---

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	tmpl, err := template.ParseFiles("index.html")
	if err != nil {
		http.Error(w, "Could not load template: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	Messages := make([]OllamaMessage, 0)

	for {
		var req ChatRequest
		err := conn.ReadJSON(&req)
		if err != nil {
			log.Println("Client disconnected:", err)
			break
		}

		err = streamOllama(conn, req.Message, &Messages)
		if err != nil {
			log.Println("Ollama error:", err)
			conn.WriteJSON(StreamResponse{Chunk: "Error: " + err.Error(), Done: true})
		}
	}
}

func streamOllama(ws *websocket.Conn, userPrompt string, messages *[]OllamaMessage) error {
	*messages = append(*messages, OllamaMessage{Role: "user", Content: userPrompt})

	const WindowSize = 10
	systemMessage := OllamaMessage{
		Role:    "system",
		Content: "You are an assistant who speaks in gangster slang.",
	}

	// Sliding Window Logic
	messagesToSend := []OllamaMessage{systemMessage}
	var recentMessages []OllamaMessage
	if len(*messages) > WindowSize {
		recentMessages = (*messages)[len(*messages)-WindowSize:]
	} else {
		recentMessages = *messages
	}
	messagesToSend = append(messagesToSend, recentMessages...)

	reqBody := OllamaRequest{
		Model:    "gemma3:1b", // Ensure this model exists!
		Messages: messagesToSend,
		Stream:   true,
		Options: map[string]interface{}{
			"temperature": 0.5,
			"top_k":       1,
			"top_p":       0.9,
		},
	}

	jsonPayload, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", OllamaAPIURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var fullBotResponse strings.Builder

	for scanner.Scan() {
		line := scanner.Bytes()

		var responseObj map[string]interface{}
		if err := json.Unmarshal(line, &responseObj); err != nil {
			continue
		}

		if content, ok := responseObj["message"].(map[string]interface{}); ok {
			if text, ok := content["content"].(string); ok {
				ws.WriteJSON(StreamResponse{Chunk: text, Done: false})
				fullBotResponse.WriteString(text)
			}
		}
	}

	// Check for scanner errors (e.g., connection cut mid-stream)
	if err := scanner.Err(); err != nil {
		log.Println("Stream scan error:", err)
	}

	*messages = append(*messages, OllamaMessage{
		Role:    "assistant",
		Content: fullBotResponse.String(),
	})

	return ws.WriteJSON(StreamResponse{Chunk: "", Done: true})
}
