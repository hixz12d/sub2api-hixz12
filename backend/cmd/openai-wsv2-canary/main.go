package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type options struct {
	URL       string
	KeyFile   string
	Model     string
	Prompt    string
	RequestID string
	Timeout   time.Duration
}

type result struct {
	RequestID       string `json:"request_id"`
	ClientRequestID string `json:"client_request_id"`
	TerminalEvent   string `json:"terminal_event"`
	ResponseID      string `json:"response_id,omitempty"`
	OutputText      string `json:"output_text,omitempty"`
}

type responseEvent struct {
	Type     string `json:"type"`
	Delta    string `json:"delta"`
	Response struct {
		ID    string `json:"id"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	var opts options
	flag.StringVar(&opts.URL, "url", "ws://127.0.0.1:8100/v1/responses", "Responses WebSocket URL")
	flag.StringVar(&opts.KeyFile, "key-file", "", "mode-0600 file containing one API key")
	flag.StringVar(&opts.Model, "model", "gpt-5.4", "model to request")
	flag.StringVar(&opts.Prompt, "prompt", "Reply with exactly: pong", "short canary prompt")
	flag.StringVar(&opts.RequestID, "request-id", "", "optional UUID for X-Request-ID")
	flag.DurationVar(&opts.Timeout, "timeout", 90*time.Second, "end-to-end timeout")
	flag.Parse()

	if err := run(context.Background(), opts, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "wsv2 canary failed: %v\n", err)
		os.Exit(1)
	}
}

func run(parent context.Context, opts options, output io.Writer) error {
	if opts.KeyFile == "" {
		return errors.New("-key-file is required")
	}
	if opts.Timeout <= 0 || opts.Timeout > 5*time.Minute {
		return errors.New("-timeout must be between 0 and 5m")
	}
	parsedURL, err := url.Parse(opts.URL)
	if err != nil || (parsedURL.Scheme != "ws" && parsedURL.Scheme != "wss") || parsedURL.Host == "" {
		return errors.New("-url must be an absolute ws or wss URL")
	}
	if strings.TrimSpace(opts.Model) == "" || strings.TrimSpace(opts.Prompt) == "" {
		return errors.New("-model and -prompt must be non-empty")
	}

	apiKey, err := readAPIKey(opts.KeyFile)
	if err != nil {
		return err
	}
	requestID := strings.TrimSpace(opts.RequestID)
	if requestID == "" {
		requestID, err = newUUID()
		if err != nil {
			return fmt.Errorf("generate request id: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(parent, opts.Timeout)
	defer cancel()

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+apiKey)
	headers.Set("User-Agent", "codex_cli_rs/0.1.0")
	headers.Set("OpenAI-Beta", "responses_websockets=2025-02-01")
	headers.Set("X-Request-ID", requestID)

	conn, resp, err := websocket.Dial(ctx, opts.URL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		if resp != nil {
			return fmt.Errorf("websocket handshake returned HTTP %d: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("websocket handshake: %w", err)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(4 << 20)

	clientRequestID := strings.TrimSpace(resp.Header.Get("X-Client-Request-ID"))
	if clientRequestID == "" {
		return errors.New("websocket handshake omitted X-Client-Request-ID")
	}

	payload, err := json.Marshal(map[string]any{
		"type":   "response.create",
		"model":  strings.TrimSpace(opts.Model),
		"input":  strings.TrimSpace(opts.Prompt),
		"stream": true,
	})
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		return fmt.Errorf("send response.create: %w", err)
	}

	var responseID string
	var text strings.Builder
	for {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("read response event: %w", err)
		}
		if messageType != websocket.MessageText && messageType != websocket.MessageBinary {
			continue
		}

		var event responseEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("decode response event: %w", err)
		}
		if event.Response.ID != "" {
			responseID = event.Response.ID
		}
		if event.Type == "response.output_text.delta" {
			_, _ = text.WriteString(event.Delta)
		}

		switch event.Type {
		case "response.completed", "response.done":
			_ = conn.Close(websocket.StatusNormalClosure, "canary complete")
			return json.NewEncoder(output).Encode(result{
				RequestID:       requestID,
				ClientRequestID: clientRequestID,
				TerminalEvent:   event.Type,
				ResponseID:      responseID,
				OutputText:      text.String(),
			})
		case "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
			failure := event.Error
			if failure == nil {
				failure = event.Response.Error
			}
			if failure != nil {
				return fmt.Errorf("upstream terminal event=%q code=%q message=%q", event.Type, failure.Code, failure.Message)
			}
			return fmt.Errorf("upstream terminal event=%q", event.Type)
		}
	}
}

func readAPIKey(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat key file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("key file must be a regular file")
	}
	// Windows os.FileMode does not model ACLs and commonly reports 0666 even
	// when the file was created with 0600. Keep the Unix safety check where
	// permission bits are authoritative; Windows deployments must secure the
	// key file with an appropriate ACL.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("key file permissions must not exceed 0600 (got %04o)", info.Mode().Perm())
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read key file: %w", err)
	}
	key := strings.TrimSpace(string(contents))
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return "", errors.New("key file must contain exactly one non-empty line")
	}
	return key, nil
}

func newUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := make([]byte, 32)
	hex.Encode(encoded, raw[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32]), nil
}
