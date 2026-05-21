package auth

import "time"

type User struct {
	ID              int64
	Email           string
	Nickname        string
	Bio             string
	AvatarObjectKey string
	Status          string
	GroupCode       string
	GroupMultiplier string
	TokenVersion    int
	DefaultLocale   string
	Theme           string
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

type UpdateProfileRequest struct {
	UserID          int64
	Nickname        string
	Bio             string
	AvatarObjectKey string
	DefaultLocale   string
	Theme           string
}
