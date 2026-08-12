package domain

type GatewayConfig struct {
	GatewayID           string `json:"gatewayId"`
	Endpoint            string `json:"endpoint"`
	APIKey              string `json:"apiKey,omitempty"`
	APIPollingEnabled   bool   `json:"apiPollingEnabled"`
	SendIntervalSeconds int    `json:"sendIntervalSeconds"`
	SendTimeoutSeconds  int    `json:"sendTimeoutSeconds"`
	// Longest a device may go without a stored reading while its register
	// values are not moving. See app.DefaultIdleHeartbeat for what it is for.
	IdleHeartbeatSeconds int `json:"idleHeartbeatSeconds"`
}
