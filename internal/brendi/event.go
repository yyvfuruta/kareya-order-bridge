package brendi

// Event e um evento pendente retornado por GET /v1/events:polling.
//
// O padrao Open Delivery chama o identificador de "eventId", mas a Brendi
// valida o acknowledgment com "id". Os dois campos sao aceitos e Key()
// resolve qual usar.
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

// Key devolve o identificador do evento, seja ele "id" ou "eventId".
func (e Event) Key() string {
	if e.ID != "" {
		return e.ID
	}
	return e.EventID
}

// AckEvent e o formato aceito por POST /v1/events/acknowledgment.
type AckEvent struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"orderId"`
	EventType EventType `json:"eventType"`
}

// Ack converte o evento recebido no payload de confirmacao.
func (e Event) Ack() AckEvent {
	return AckEvent{ID: e.Key(), OrderID: e.OrderID, EventType: e.EventType}
}
