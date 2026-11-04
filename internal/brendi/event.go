package brendi

// Event is a pending event returned by GET /v1/events:polling.
//
// The Open Delivery standard calls the identifier "eventId", but Brendi
// validates the acknowledgment with "id". Both fields are accepted and Key()
// resolves which one to use.
type Event struct {
	ID           string    `json:"id,omitempty"`
	EventID      string    `json:"eventId,omitempty"`
	EventType    EventType `json:"eventType"`
	OrderID      string    `json:"orderId"`
	OrderURL     string    `json:"orderURL,omitempty"`
	CreatedAt    DateTime  `json:"createdAt"`
	SourceAppID  string    `json:"sourceAppId,omitempty"`
	VirtualBrand string    `json:"virtualBrand,omitempty"`
}

// Key returns the event identifier, whether it is "id" or "eventId".
func (e Event) Key() string {
	if e.ID != "" {
		return e.ID
	}
	return e.EventID
}

// AckEvent is the format accepted by POST /v1/events/acknowledgment.
type AckEvent struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"orderId"`
	EventType EventType `json:"eventType"`
}

// Ack converts the received event into the acknowledgment payload.
func (e Event) Ack() AckEvent {
	return AckEvent{ID: e.Key(), OrderID: e.OrderID, EventType: e.EventType}
}
