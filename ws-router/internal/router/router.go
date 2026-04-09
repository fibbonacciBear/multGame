package router

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

type Config struct {
	Port         string
	JWTSecret    string
	BackendPort  string
	DialTimeout  time.Duration
	WriteTimeout time.Duration
}

type Handler struct {
	cfg      Config
	logger   *log.Logger
	upgrader websocket.Upgrader
	dialer   websocket.Dialer
}

type routeClaims struct {
	LobbyID string `json:"lobby_id"`
	PodIP   string `json:"pod_ip"`
	jwt.RegisteredClaims
}

func LoadConfig() Config {
	return Config{
		Port:         envOrDefault("PORT", "8082"),
		JWTSecret:    envOrDefault("JWT_SECRET", "dev-secret"),
		BackendPort:  envOrDefault("BACKEND_PORT", "8080"),
		DialTimeout:  envDuration("DIAL_TIMEOUT", 3*time.Second),
		WriteTimeout: envDuration("WRITE_TIMEOUT", 5*time.Second),
	}
}

func New(cfg Config, logger *log.Logger) *Handler {
	return &Handler{
		cfg:    cfg,
		logger: logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
		dialer: websocket.Dialer{
			HandshakeTimeout: cfg.DialTimeout,
		},
	}
}

func (h *Handler) HandleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) HandleWS(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/ws/")
	token = strings.TrimSpace(token)
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	claims, err := h.parseToken(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	backendURL := url.URL{
		Scheme: "ws",
		Host:   net.JoinHostPort(claims.PodIP, h.cfg.BackendPort),
		Path:   "/ws",
	}
	query := backendURL.Query()
	query.Set("lobby", claims.LobbyID)
	query.Set("token", token)
	backendURL.RawQuery = query.Encode()

	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.DialTimeout)
	defer cancel()

	backendConn, _, err := h.dialer.DialContext(ctx, backendURL.String(), nil)
	if err != nil {
		h.logger.Printf("backend dial failed for %s: %v", claims.PodIP, err)
		http.Error(w, "pod unavailable", http.StatusBadGateway)
		return
	}
	defer backendConn.Close()

	clientConn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Printf("client upgrade failed: %v", err)
		return
	}
	defer clientConn.Close()

	errCh := make(chan error, 2)
	var once sync.Once
	closeClient := func(code int, text string) {
		once.Do(func() {
			message := websocket.FormatCloseMessage(code, text)
			_ = clientConn.WriteControl(websocket.CloseMessage, message, time.Now().Add(h.cfg.WriteTimeout))
		})
	}

	go pumpWebSocket(errCh, backendConn, clientConn)
	go pumpWebSocket(errCh, clientConn, backendConn)

	err = <-errCh
	logSecondaryPumpError(h.logger, errCh)
	if isNormalClose(err) {
		closeClient(websocket.CloseNormalClosure, "")
		return
	}

	closeClient(4001, "pod_unavailable")
}

func pumpWebSocket(errCh chan<- error, src, dst *websocket.Conn) {
	for {
		messageType, payload, err := src.ReadMessage()
		if err != nil {
			errCh <- err
			return
		}
		if err := dst.WriteMessage(messageType, payload); err != nil {
			errCh <- err
			return
		}
	}
}

func logSecondaryPumpError(logger *log.Logger, errCh <-chan error) {
	select {
	case err := <-errCh:
		if !isNormalClose(err) {
			logger.Printf("secondary websocket pump error: %v", err)
		}
	default:
	}
}

func (h *Handler) parseToken(token string) (*routeClaims, error) {
	claims := &routeClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims.PodIP == "" || claims.LobbyID == "" {
		return nil, fmt.Errorf("missing routing claims")
	}
	return claims, nil
}

func isNormalClose(err error) bool {
	return websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}
