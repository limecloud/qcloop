package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源（生产环境应该限制）
	},
}

// WSHub WebSocket 连接管理中心
type WSHub struct {
	clients    map[*WSClient]bool
	broadcast  chan []byte
	register   chan *WSClient
	unregister chan *WSClient
	mu         sync.RWMutex
}

// WSClient WebSocket 客户端
type WSClient struct {
	hub    *WSHub
	conn   *websocket.Conn
	send   chan []byte
	jobID  string
}

// NewWSHub 创建 WebSocket Hub
func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

// Run 运行 WebSocket Hub
func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("WebSocket client registered: %s", client.jobID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("WebSocket client unregistered: %s", client.jobID)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastJobUpdate 广播批次更新
func (h *WSHub) BroadcastJobUpdate(jobID string, data interface{}) {
	message := map[string]interface{}{
		"type":   "job_update",
		"job_id": jobID,
		"data":   data,
		"time":   time.Now().Unix(),
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling job update: %v", err)
		return
	}

	h.mu.RLock()
	for client := range h.clients {
		if client.jobID == jobID || client.jobID == "" {
			select {
			case client.send <- jsonData:
			default:
				// 客户端发送缓冲区已满，跳过
			}
		}
	}
	h.mu.RUnlock()
}

// BroadcastItemUpdate 广播测试项更新
func (h *WSHub) BroadcastItemUpdate(jobID string, itemID string, data interface{}) {
	message := map[string]interface{}{
		"type":    "item_update",
		"job_id":  jobID,
		"item_id": itemID,
		"data":    data,
		"time":    time.Now().Unix(),
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling item update: %v", err)
		return
	}

	h.mu.RLock()
	for client := range h.clients {
		if client.jobID == jobID || client.jobID == "" {
			select {
			case client.send <- jsonData:
			default:
				// 客户端发送缓冲区已满，跳过
			}
		}
	}
	h.mu.RUnlock()
}

// handleWebSocket 处理 WebSocket 连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	jobID := r.URL.Query().Get("job_id")

	client := &WSClient{
		hub:   s.wsHub,
		conn:  conn,
		send:  make(chan []byte, 256),
		jobID: jobID,
	}

	client.hub.register <- client

	// 启动读写协程
	go client.writePump()
	go client.readPump()
}

// readPump 从 WebSocket 读取消息
func (c *WSClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}

// writePump 向 WebSocket 写入消息
func (c *WSClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// 批量发送队列中的其他消息
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
