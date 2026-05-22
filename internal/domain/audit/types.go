package audit

import "time"

type Log struct {
	ID         int64
	ActorType  string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Result     string
	Metadata   map[string]any
	IPAddr     string
	UserAgent  string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type RecordRequest struct {
	ActorType  string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Result     string
	Metadata   map[string]any
	IPAddr     string
	UserAgent  string
}

type ListRequest struct {
	Page        int
	PageSize    int
	ActorType   string
	ActorID     string
	Action      string
	TargetType  string
	TargetID    string
	Result      string
	CreatedFrom time.Time
	CreatedTo   time.Time
}

type ListPage struct {
	Items    []Log `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int   `json:"total"`
}
