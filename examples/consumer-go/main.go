// Command consumer-go demonstrates how to authenticate with the AI Proxy
// using the encrypted X-Auth header (recommended for public APIs).
//
// Usage:
//   export CLIENT_ID=sk-your-client-id
//   export ENCRYPTION_KEY=your-base64-encoded-32-byte-key
//   go run examples/consumer-go/main.go
//
// Or with a custom proxy URL:
//   PROXY_URL=http://localhost:18080 go run examples/consumer-go/main.go
package main

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ─── Configuration ────────────────────────────────────────────

var (
	proxyURL      = getEnv("PROXY_URL", "http://localhost:18080")
	clientID      = os.Getenv("CLIENT_ID")
	encryptionKey = os.Getenv("ENCRYPTION_KEY")
	httpClient    = &http.Client{Timeout: 30 * time.Second}
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ─── X-Auth Header Generation ─────────────────────────────────

// GenerateXAuth creates the X-Auth header value by AES-256-GCM encrypting
// "client_id:timestamp:nonce" with the consumer's encryption key.
//
// The output format matches the server's encryption.Encrypt function:
//
//	base64_urlsafe_no_pad( iv (12 bytes) || ciphertext || auth_tag (16 bytes) )
//
// The server decrypts by:
//  1. Base64 URL-safe decoding the header
//  2. Extracting the first 12 bytes as the IV
//  3. AES-GCM decrypting the remainder with the client's encryption_key
//  4. Parsing "client_id:timestamp:nonce" from the plaintext
func GenerateXAuth(clientID, encryptionKey string, timestamp int64, nonce string) (string, error) {
	// Decode the base64 URL-safe encryption key to 32 bytes
	key, err := base64.RawURLEncoding.DecodeString(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("decode encryption key: %w", err)
	}
	if len(key) != 32 {
		return "", fmt.Errorf("invalid key length: %d (expected 32)", len(key))
	}

	// Build plaintext payload: "client_id:timestamp:nonce"
	plaintext := []byte(fmt.Sprintf("%s:%d:%s", clientID, timestamp, nonce))

	// Create AES-256-GCM cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}

	// Generate random 12-byte IV (AES-GCM standard nonce size)
	iv := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("generate iv: %w", err)
	}

	// Seal prepends the IV: output = iv || ciphertext || auth_tag
	ciphertext := gcm.Seal(iv, iv, plaintext, nil)

	// Base64 URL-safe encode without padding
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// ─── Proxy Request ────────────────────────────────────────────

// ChatRequest is the JSON body for a chat completion request.
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
}

// ChatMessage is a single message in the conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is the non-streaming proxy response.
type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// ChatCompletion sends a non-streaming chat completion request.
func ChatCompletion(model string, messages []ChatMessage) (*ChatResponse, error) {
	timestamp := time.Now().Unix()
	nonce := fmt.Sprintf("%s-%d", randomID(), timestamp)

	xAuth, err := GenerateXAuth(clientID, encryptionKey, timestamp, nonce)
	if err != nil {
		return nil, fmt.Errorf("generate x-auth: %w", err)
	}

	body := ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequest("POST", proxyURL+"/api/v1/chat/completions",
		bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-ID", clientID)
	req.Header.Set("X-Auth", xAuth)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("proxy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("proxy error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// ChatCompletionStream sends a streaming request and calls onChunk for
// each content delta. Blocks until the stream completes or errors.
func ChatCompletionStream(model string, messages []ChatMessage, onChunk func(string)) error {
	timestamp := time.Now().Unix()
	nonce := fmt.Sprintf("%s-%d", randomID(), timestamp)

	xAuth, err := GenerateXAuth(clientID, encryptionKey, timestamp, nonce)
	if err != nil {
		return fmt.Errorf("generate x-auth: %w", err)
	}

	body := ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   true,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequest("POST", proxyURL+"/api/v1/chat/completions",
		bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-ID", clientID)
	req.Header.Set("X-Auth", xAuth)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("proxy error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	// Read SSE stream: the proxy forwards the upstream's SSE output directly.
	// Format is:
	//   data: {"choices":[{"delta":{"content":"Hello"},"index":0}]}
	//   data: [DONE]
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return nil
		}

		var sse struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &sse); err != nil {
			continue // skip malformed SSE lines
		}
		for _, choice := range sse.Choices {
			if choice.Delta.Content != "" {
				onChunk(choice.Delta.Content)
			}
			if choice.FinishReason != nil && *choice.FinishReason == "stop" {
				return nil
			}
		}
	}
	return scanner.Err()
}

// randomID generates a short random identifier for use in nonces.
func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// ─── Main ─────────────────────────────────────────────────────

func main() {
	if clientID == "" || encryptionKey == "" {
		fmt.Fprintln(os.Stderr, "Please set CLIENT_ID and ENCRYPTION_KEY environment variables.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  export CLIENT_ID=sk-your-client-id")
		fmt.Fprintln(os.Stderr, "  export ENCRYPTION_KEY=your-base64-encoded-32-byte-key")
		fmt.Fprintln(os.Stderr, "  go run examples/consumer-go/main.go")
		os.Exit(1)
	}

	fmt.Println("AI Proxy Consumer SDK — Go Example")
	fmt.Printf("  Proxy URL:  %s\n", proxyURL)
	fmt.Printf("  Client ID:  %s...\n", truncate(clientID, 20))
	fmt.Println()

	// ── Non-streaming request ────────────────────────────────
	fmt.Println("── Non-streaming chat completion ──")

	result, err := ChatCompletion("gpt-4", []ChatMessage{
		{Role: "user", Content: "Hello! Tell me a short joke."},
	})
	if err != nil {
		fmt.Printf("  ✗ Failed: %v\n", err)
	} else {
		content := ""
		if len(result.Choices) > 0 {
			content = result.Choices[0].Message.Content
		}
		fmt.Printf("  Response: %s\n", content)
	}

	// ── Streaming request ────────────────────────────────────
	fmt.Println()
	fmt.Println("── Streaming chat completion ──")
	fmt.Print("  Response: ")

	err = ChatCompletionStream("gpt-4", []ChatMessage{
		{Role: "user", Content: "Count from 1 to 5."},
	}, func(chunk string) {
		fmt.Print(chunk)
	})
	if err != nil {
		fmt.Printf("\n  ✗ Failed: %v\n", err)
	} else {
		fmt.Println()
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
