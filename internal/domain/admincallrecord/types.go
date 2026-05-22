package admincallrecord

import "time"

type Record struct {
	TaskID                    string     `json:"task_id"`
	UserID                    int64      `json:"user_id"`
	APIKeyID                  *int64     `json:"api_key_id"`
	SourceChannel             string     `json:"source_channel"`
	TaskType                  string     `json:"task_type"`
	Status                    string     `json:"status"`
	Provider                  string     `json:"provider"`
	AbstractModel             string     `json:"abstract_model"`
	Quality                   string     `json:"quality"`
	RequestedOutputImageCount int        `json:"requested_output_image_count"`
	SuccessOutputImageCount   int        `json:"success_output_image_count"`
	ReferenceImageCount       int        `json:"reference_image_count"`
	EstimatedPoints           string     `json:"estimated_points"`
	ActualPoints              string     `json:"actual_points"`
	ErrorCode                 *string    `json:"error_code"`
	ErrorMessage              *string    `json:"error_message"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
	StartedAt                 *time.Time `json:"started_at"`
	FinishedAt                *time.Time `json:"finished_at"`
	AttemptCount              int        `json:"attempt_count"`
}

type ListRequest struct {
	Page          int
	PageSize      int
	Status        string
	Provider      string
	SourceChannel string
	UserID        int64
	TaskID        string
	CreatedFrom   time.Time
	CreatedTo     time.Time
}

type ListPage struct {
	Items    []Record `json:"items"`
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
	Total    int      `json:"total"`
}
