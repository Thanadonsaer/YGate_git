package domain

type DeliveryLog struct {
	ID             int64  `json:"id"`
	IdempotencyKey string `json:"idempotencyKey"`
	Status         string `json:"status"`
	Attempts       int    `json:"attempts"`
	LastHTTPStatus int    `json:"lastHttpStatus,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	LastResponse   string `json:"lastResponse,omitempty"`
	CreatedAt      int64  `json:"createdAt"`
	DeliveredAt    int64  `json:"deliveredAt,omitempty"`
}
