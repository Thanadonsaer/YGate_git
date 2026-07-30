package domain

type PollLog struct {
	ID             int64  `json:"id"`
	ConnectionID   int64  `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Status         string `json:"status"`
	Message        string `json:"message,omitempty"`
	Detail         string `json:"detail,omitempty"`
	CreatedAt      int64  `json:"createdAt"`
}
