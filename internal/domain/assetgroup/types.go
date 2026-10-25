package assetgroup

import "time"

type Group struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"user_id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type CreateRequest struct {
	Name string `json:"name"`
}
type UpdateRequest struct {
	Name            string `json:"name"`
	ExpectedVersion int64  `json:"expected_version"`
}
type DeleteRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}
