package gateway

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testServer() (*Server, *httptest.Server) {
	srv := NewServer(Config{ServiceToken: "service-token", LobeAPIBaseURL: "http://127.0.0.1:1"})
	srv.authTimeout = time.Second
	srv.heartbeatTimeout = time.Second
	srv.cleanupDelay = time.Second
	httpSrv := httptest.NewServer(srv.Routes())
	return srv, httpSrv
}

func TestHealth(t *testing.T) {
	_, ts := testServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "OK" {
		t.Fatalf("unexpected health response: %d %q", resp.StatusCode, body)
	}
}

func TestOperationsAuthAndStatus(t *testing.T) {
	_, ts := testServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/operations/status?operationId=op-auth")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	postJSON(t, ts.URL+"/api/operations/init", map[string]any{"operationId": "op-auth", "userId": "user-1"}, http.StatusOK)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/operations/status?operationId=op-auth", nil)
	req.Header.Set("Authorization", "Bearer service-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var status struct {
		ClientConnected bool   `json:"clientConnected"`
		Status          string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.ClientConnected || status.Status != "running" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestWebSocketPushAndResume(t *testing.T) {
	_, ts := testServer()
	defer ts.Close()
	postJSON(t, ts.URL+"/api/operations/init", map[string]any{"operationId": "op-ws", "userId": "user-1"}, http.StatusOK)

	ws := dialWebSocket(t, ts.URL, "/ws?operationId=op-ws")
	defer ws.close()
	ws.writeJSON(t, map[string]any{"type": "auth", "token": "service-token"})
	if msg := ws.readJSON(t); msg["type"] != "auth_success" {
		t.Fatalf("expected auth_success, got %+v", msg)
	}

	postJSON(t, ts.URL+"/api/operations/push-event", map[string]any{
		"operationId": "op-ws",
		"event": map[string]any{
			"data":        map[string]any{"chunkType": "text", "content": "hello"},
			"operationId": "op-ws",
			"stepIndex":   0,
			"timestamp":   time.Now().UnixMilli(),
			"type":        "stream_chunk",
		},
	}, http.StatusOK)

	msg := ws.readJSON(t)
	if msg["type"] != "agent_event" || msg["id"] != "1" {
		t.Fatalf("unexpected event: %+v", msg)
	}
	ws.close()

	postJSON(t, ts.URL+"/api/operations/push-event", map[string]any{
		"operationId": "op-ws",
		"event": map[string]any{
			"data":        map[string]any{"chunkType": "text", "content": "again"},
			"operationId": "op-ws",
			"stepIndex":   0,
			"timestamp":   time.Now().UnixMilli(),
			"type":        "stream_chunk",
		},
	}, http.StatusOK)

	ws2 := dialWebSocket(t, ts.URL, "/ws?operationId=op-ws")
	defer ws2.close()
	ws2.writeJSON(t, map[string]any{"type": "auth", "token": "service-token"})
	_ = ws2.readJSON(t)
	ws2.writeJSON(t, map[string]any{"type": "resume", "lastEventId": "1"})
	resumed := ws2.readJSON(t)
	if resumed["type"] != "agent_event" || resumed["id"] != "2" {
		t.Fatalf("unexpected resumed event: %+v", resumed)
	}
}

func TestConfirmationAndInput(t *testing.T) {
	_, ts := testServer()
	defer ts.Close()
	postJSON(t, ts.URL+"/api/operations/init", map[string]any{"operationId": "op-pending", "userId": "user-1"}, http.StatusOK)

	ws := dialWebSocket(t, ts.URL, "/ws?operationId=op-pending")
	defer ws.close()
	ws.writeJSON(t, map[string]any{"type": "auth", "token": "service-token"})
	_ = ws.readJSON(t)

	confirmationDone := make(chan map[string]any, 1)
	go func() {
		confirmationDone <- postJSONBody(t, ts.URL+"/api/operations/request-confirmation", map[string]any{
			"operationId": "op-pending",
			"timeout":     1000,
			"toolCallId":  "tool-1",
			"tool": map[string]any{
				"apiName":    "file.delete",
				"arguments":  "{}",
				"identifier": "file.delete",
				"name":       "delete",
			},
		}, http.StatusOK)
	}()
	request := ws.readJSON(t)
	if request["type"] != "tool_confirmation_request" || request["toolCallId"] != "tool-1" {
		t.Fatalf("unexpected confirmation request: %+v", request)
	}
	ws.writeJSON(t, map[string]any{"type": "tool_confirmation", "toolCallId": "tool-1", "approved": true})
	if result := <-confirmationDone; result["approved"] != true {
		t.Fatalf("unexpected confirmation result: %+v", result)
	}

	inputDone := make(chan map[string]any, 1)
	go func() {
		inputDone <- postJSONBody(t, ts.URL+"/api/operations/request-input", map[string]any{"operationId": "op-pending", "prompt": "name?", "timeout": 1000}, http.StatusOK)
	}()
	inputReq := ws.readJSON(t)
	requestID, _ := inputReq["requestId"].(string)
	if inputReq["type"] != "input_request" || requestID == "" {
		t.Fatalf("unexpected input request: %+v", inputReq)
	}
	ws.writeJSON(t, map[string]any{"type": "user_input", "requestId": requestID, "content": "alice"})
	if result := <-inputDone; result["content"] != "alice" {
		t.Fatalf("unexpected input result: %+v", result)
	}
}

func postJSON(t *testing.T, url string, body any, expected int) {
	t.Helper()
	_ = postJSONBody(t, url, body, expected)
}

func postJSONBody(t *testing.T, url string, body any, expected int) map[string]any {
	t.Helper()
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer service-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != expected {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected %d, got %d: %s", expected, resp.StatusCode, data)
	}
	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return result
}

type testWS struct {
	br   *bufio.Reader
	conn net.Conn
}

func dialWebSocket(t *testing.T, serverURL string, path string) *testWS {
	t.Helper()
	addr := strings.TrimPrefix(serverURL, "http://")
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	key := randomWSKey(t)
	request := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("unexpected websocket status: %s", status)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	return &testWS{br: br, conn: conn}
}

func (w *testWS) writeJSON(t *testing.T, value any) {
	t.Helper()
	payload, _ := json.Marshal(value)
	frame := []byte{0x81}
	mask := []byte{1, 2, 3, 4}
	if len(payload) < 126 {
		frame = append(frame, 0x80|byte(len(payload)))
	} else {
		t.Fatalf("test payload too large")
	}
	frame = append(frame, mask...)
	for i, b := range payload {
		frame = append(frame, b^mask[i%4])
	}
	if _, err := w.conn.Write(frame); err != nil {
		t.Fatal(err)
	}
}

func (w *testWS) readJSON(t *testing.T) map[string]any {
	t.Helper()
	_ = w.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	first, err := w.br.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	if first&0x0f != wsTextMessage {
		t.Fatalf("unexpected opcode %d", first&0x0f)
	}
	second, err := w.br.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	length := int(second & 0x7f)
	if length == 126 {
		buf := make([]byte, 2)
		if _, err := io.ReadFull(w.br, buf); err != nil {
			t.Fatal(err)
		}
		length = int(buf[0])<<8 | int(buf[1])
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(w.br, payload); err != nil {
		t.Fatal(err)
	}
	var msg map[string]any
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("invalid json %q: %v", payload, err)
	}
	return msg
}

func (w *testWS) close() {
	_ = w.conn.Close()
}

func randomWSKey(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func expectedAccept(key string) string {
	sum := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}
