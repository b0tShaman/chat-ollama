# Chat-Ollama 🦙

A lightweight, real-time AI chat interface powered by **Go** and **Ollama**. It runs entirely locally but includes built-in options to share your bot over **LAN (WiFi)** or the internet via **Ngrok**.

## ✨ Features
* **Real-time Streaming:** Typewriter effect using WebSockets.
* **Context Aware:** Remembers conversation history (with sliding window memory).
* **Privacy First:** Runs 100% locally on your hardware.
* **Sharing Modes:**
    * `local`: For personal use on `localhost`.
    * `lan`: Share with devices on your WiFi.
    * `ngrok`: Share with the world via a secure tunnel.

## 🛠️ Prerequisites
1.  **[Go](https://go.dev/dl/)** (v1.21 or higher)
2.  **[Ollama](https://ollama.com/)** running locally.

## 🚀 Quick Start

### 1. Setup Model
Ensure Ollama is running and pull the model used in the code (default: `gemma3:1b`):
```bash
ollama serve
ollama pull gemma3:1b
```
### 2. Install Dependencies
```bash
go get github.com/gorilla/websocket
go get golang.ngrok.com/ngrok
```
### 3. Run the Server
You can run the server in three different modes:
#### A. Local Mode (Default) Only accessible from your computer.
```bash
go run main.go
# Open http://localhost:8080
```
#### B. LAN Mode (WiFi Sharing) Accessible by phones/laptops on the same WiFi network.
```bash
go run main.go lan
# The terminal will print your local IP, e.g., http://192.168.1.5:8080
```
#### C. Ngrok Mode (Internet Sharing) Accessible from anywhere in the world. Prerequisite: You must create an Ngrok account and export your authtoken before running.
```bash
export NGROK_AUTHTOKEN="your_token_here"
go run main.go ngrok
```