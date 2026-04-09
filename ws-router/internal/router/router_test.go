package router

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestParseToken(t *testing.T) {
	handler := New(Config{JWTSecret: "secret"}, nil)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"pod_ip":   "10.0.0.1",
		"lobby_id": "lobby-a",
		"exp":      time.Now().Add(time.Minute).Unix(),
	}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	claims, err := handler.parseToken(token)
	if err != nil {
		t.Fatalf("parseToken() error = %v", err)
	}
	if claims.PodIP != "10.0.0.1" || claims.LobbyID != "lobby-a" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestParseTokenRejectsMissingClaims(t *testing.T) {
	handler := New(Config{JWTSecret: "secret"}, nil)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Minute).Unix(),
	}).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	if _, err := handler.parseToken(token); err == nil {
		t.Fatal("parseToken() expected error, got nil")
	}
}
