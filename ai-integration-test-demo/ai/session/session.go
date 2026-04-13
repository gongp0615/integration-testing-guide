package session

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/example/ai-integration-test-demo/ai/tools"
	"github.com/gorilla/websocket"
)

type Session struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func New(conn *websocket.Conn) *Session {
	return &Session{conn: conn}
}

func (s *Session) SendCommand(params tools.SendCommandParams) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	if err := s.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return nil, err
	}

	_, msg, err := s.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(msg, &result); err != nil {
		return nil, err
	}

	log.Printf("WS ← %s", string(msg))
	return result, nil
}
