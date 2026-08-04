package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUserJSONOmitsPassword(t *testing.T) {
	u := User{
		ID:        1,
		Username:  "alice",
		Password:  "bcrypt-digest-must-not-leak",
		Role:      "user",
		CreatedAt: time.Unix(1000, 0).UTC(),
		UpdatedAt: time.Unix(2000, 0).UTC(),
	}
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["password"]; ok {
		t.Fatalf("password field must not appear in JSON: %s", string(b))
	}
	if m["username"] != "alice" {
		t.Fatalf("username: got %v", m["username"])
	}
	if m["role"] != "user" {
		t.Fatalf("role: got %v", m["role"])
	}
}
