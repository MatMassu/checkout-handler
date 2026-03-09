package payments

// WebhookNotification is the payload MercadoPago POSTs to our webhook endpoint.
type WebhookNotification struct {
	ID       int64  `json:"id"`
	LiveMode bool   `json:"live_mode"`
	Type     string `json:"type"`
	Action   string `json:"action"`
	Data     struct {
		ID string `json:"id"`
	} `json:"data"`
}
