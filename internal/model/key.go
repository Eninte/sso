package model

import (
	"time"
)

type KeyStatus string

const (
	KeyStatusActive     KeyStatus = "active"
	KeyStatusDeprecated KeyStatus = "deprecated"
	KeyStatusRevoked    KeyStatus = "revoked"
)

type KeyVersion struct {
	ID         string     `json:"id" db:"id"`
	PublicKey  []byte     `json:"public_key" db:"public_key"`
	PrivateKey []byte     `json:"-" db:"private_key"`
	Status     KeyStatus  `json:"status" db:"status"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty" db:"expires_at"`
}

func (k *KeyVersion) IsActive() bool {
	return k.Status == KeyStatusActive
}

func (k *KeyVersion) IsDeprecated() bool {
	return k.Status == KeyStatusDeprecated
}

func (k *KeyVersion) IsRevoked() bool {
	return k.Status == KeyStatusRevoked
}

func (k *KeyVersion) CanVerify() bool {
	if k.Status == KeyStatusRevoked {
		return false
	}
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return false
	}
	return true
}
