package auth

import "time"

type User struct {
	ID              int64
	Email           string
	Nickname        string
	Status          string
	GroupCode       string
	GroupMultiplier string
	TokenVersion    int
	CreatedAt       time.Time
}

type Session struct {
	AccessToken           string
	AccessTokenExpiresAt  time.Time
	RefreshToken          string
	RefreshTokenExpiresAt time.Time
	RefreshCookieName     string
	SessionID             string
	SessionFamilyID       string
}
