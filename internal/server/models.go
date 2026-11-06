package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

var ErrConflict = errors.New("invalid alert transition")
var ErrNotFound = errors.New("not found")

type Profile struct {
	ID           string    `json:"id"`
	FullName     string    `json:"full_name"`
	BirthDate    string    `json:"birth_date"`
	Address      string    `json:"address"`
	Apartment    string    `json:"apartment"`
	Entrance     string    `json:"entrance"`
	Floor        string    `json:"floor"`
	AccessNotes  string    `json:"access_notes"`
	ContactName  string    `json:"contact_name"`
	ContactPhone string    `json:"contact_phone"`
	Conditions   string    `json:"conditions"`
	Allergies    string    `json:"allergies"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Location struct {
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Accuracy  float64   `json:"accuracy"`
	Captured  time.Time `json:"captured_at"`
}

type Alert struct {
	ID             string     `json:"id"`
	Source         string     `json:"source"`
	Status         string     `json:"status"`
	Profile        Profile    `json:"profile"`
	Location       *Location  `json:"location"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeadlineAt     *time.Time `json:"deadline_at"`
	ReceivedAt     *time.Time `json:"received_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at"`
	ClosedAt       *time.Time `json:"closed_at"`
	Events         []Event    `json:"events"`
}

type Event struct {
	ID        int64     `json:"id"`
	AlertID   string    `json:"alert_id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
}

type Dashboard struct {
	Profile    *Profile  `json:"profile,omitempty"`
	Alerts     []Alert   `json:"alerts"`
	Cursor     int64     `json:"cursor"`
	ServerTime time.Time `json:"server_time"`
	Mode       string    `json:"mode"`
}

type CreateRequest struct {
	Source          string    `json:"source"`
	ClientRequestID string    `json:"client_request_id"`
	WaitSeconds     int       `json:"wait_seconds"`
	Location        *Location `json:"location"`
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func active(status string) bool {
	return status == "awaiting_response" || status == "received" || status == "acknowledged"
}
