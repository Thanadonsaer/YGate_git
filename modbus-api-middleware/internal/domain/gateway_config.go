package domain

type GatewayConfig struct {
	GatewayID           string `json:"gatewayId"`
	Endpoint            string `json:"endpoint"`
	APIKey              string `json:"apiKey,omitempty"`
	APIPollingEnabled   bool   `json:"apiPollingEnabled"`
	SendIntervalSeconds int    `json:"sendIntervalSeconds"`
	SendTimeoutSeconds  int    `json:"sendTimeoutSeconds"`
}
