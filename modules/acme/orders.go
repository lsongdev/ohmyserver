package acme

import (
	"errors"
	"time"
)

var ErrIssuanceInProgress = errors.New("an issuance is already in progress for this certificate")

type Order struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"account_id"`
	OrderURL  string `json:"order_url"`
	Domains   string `json:"domains"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Identifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Authorization struct {
	ID         int64       `json:"id"`
	OrderID    int64       `json:"order_id"`
	AuthzURL   string      `json:"authz_url"`
	Status     string      `json:"status"`
	Identifier Identifier  `json:"identifier"`
	Expires    time.Time   `json:"expires"`
	Challenges []Challenge `json:"challenges,omitempty"`
	CreatedAt  string      `json:"created_at"`
}

type Challenge struct {
	ID              int64     `json:"id"`
	AuthorizationID int64     `json:"authorization_id"`
	ChallengeURL    string    `json:"challenge_url"`
	Type            string    `json:"type"`
	Status          string    `json:"status"`
	Token           string    `json:"token"`
	KeyAuth         string    `json:"-"`
	Validated       time.Time `json:"validated"`
	Error           string    `json:"error"`
	CreatedAt       string    `json:"created_at"`
}

type Issuance struct {
	ID            int64     `json:"id"`
	CertificateID int64     `json:"certificate_id"`
	AccountID     int64     `json:"account_id"`
	OrderID       int64     `json:"order_id"`
	Domains       string    `json:"domains"`
	ChallengeType string    `json:"challenge_type"`
	DNSTarget     string    `json:"dns_target,omitempty"`
	Status        string    `json:"status"`
	Step          string    `json:"step"`
	Error         string    `json:"error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type IssuanceRequest struct {
	Domains       string `json:"domains"`
	AccountID     int64  `json:"account_id"`
	ChallengeType string `json:"challenge_type"`
	DNSTarget     string `json:"dns_target,omitempty"`
}
