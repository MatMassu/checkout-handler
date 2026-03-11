package domain

// Order status constants — all valid states and their transitions:
//
//	pending → paid       (payment approved by MercadoPago webhook)
//	pending → failed     (payment rejected or cancelled by MercadoPago webhook)
//	pending → cancelled  (explicit cancellation by user)
//	pending → expired    (expiry worker: expires_at passed with no payment)
//
// paid, failed, cancelled, and expired are terminal — no further transitions allowed.
const (
	StatusPending   = "pending"
	StatusPaid      = "paid"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusExpired   = "expired"
)
