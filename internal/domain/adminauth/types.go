package adminauth

import "time"

type AdminUser struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type LoginRequest struct {
	Email    string
	Password string
}

type Session struct {
	AccessToken          string
	AccessTokenExpiresAt time.Time
	SessionID            string
	AdminID              int64
	Email                string
	Role                 string
	Status               string
}
