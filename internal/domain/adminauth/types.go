package adminauth

import "time"

type AdminUser struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AdminListRequest struct {
	Page     int
	PageSize int
	Query    string
	Role     string
	Status   string
}

type AdminListPage struct {
	Items    []AdminUser `json:"items"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int         `json:"total"`
}

type AdminCreateRequest struct {
	Email    string
	Password string
	Role     string
	Status   string
}

type AdminUpdateRequest struct {
	AdminID        int64
	OperatorID     int64
	Role           string
	Status         string
	RoleProvided   bool
	StatusProvided bool
}

type AdminPasswordResetRequest struct {
	AdminID     int64
	NewPassword string
}

type AdminDeleteRequest struct {
	AdminID    int64
	OperatorID int64
}

type LoginRequest struct {
	Email    string
	Password string
}

type Session struct {
	AccessToken           string
	AccessTokenExpiresAt  time.Time
	RefreshToken          string
	RefreshTokenExpiresAt time.Time
	SessionID             string
	SessionFamilyID       string
	AdminID               int64
	Email                 string
	Role                  string
	Status                string
}
