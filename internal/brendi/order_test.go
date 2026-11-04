package brendi

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return data
}

// Real (anonymized) payload from the Brendi API: 38.89 order with delivery and Pix.
func TestOrderParsing(t *testing.T) {
	var order Order
	if err := json.Unmarshal(readFixture(t, "order.json"), &order); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if order.ID != "ababf637-6a62-4922-8c1a-4b774072d930" {
		t.Errorf("id = %q", order.ID)
	}
	if order.DisplayID != "661009" {
		t.Errorf("displayId = %q", order.DisplayID)
	}
	if order.Type != OrderTypeDelivery {
		t.Errorf("type = %q", order.Type)
	}
	if order.LastEvent != EventDelivered {
		t.Errorf("lastEvent = %q", order.LastEvent)
	}
	if got := order.TotalAmount(); got != 38.89 {
		t.Errorf("total = %v", got)
	}
	if !order.CreatedAt.Valid() || !order.CreatedAt.Equal(time.Date(2026, 11, 4, 20, 28, 29, 0, time.UTC)) {
		t.Errorf("createdAt = %v", order.CreatedAt)
	}

	if len(order.Items) != 1 {
		t.Fatalf("items = %d", len(order.Items))
	}
	item := order.Items[0]
	if item.Name != "Donburi katsudon" || item.Quantity != 1 || item.TotalPrice.Value != 37.03 {
		t.Errorf("item = %+v", item)
	}
	if item.Unit != UnitUN {
		t.Errorf("item.unit = %q", item.Unit)
	}

	if len(order.OtherFees) != 2 {
		t.Fatalf("otherFees = %d", len(order.OtherFees))
	}
	if order.OtherFees[0].Type != FeeDelivery || order.OtherFees[0].ReceivedBy != ReceivedByMerchant {
		t.Errorf("fee = %+v", order.OtherFees[0])
	}

	if order.Customer == nil || order.Customer.Phone.Number != "5511999999999" {
		t.Errorf("customer = %+v", order.Customer)
	}
	if order.Delivery == nil || !order.Delivery.EstimatedDeliveryDateTime.Valid() {
		t.Fatalf("delivery = %+v", order.Delivery)
	}
	if order.Delivery.DeliveryAddress.City != "Curitiba" || order.Delivery.DeliveryAddress.Coordinates.Latitude != -25.4 {
		t.Errorf("address = %+v", order.Delivery.DeliveryAddress)
	}
	if order.Delivery.DeliveryFee == nil || order.Delivery.DeliveryFee.Value != 4.49 {
		t.Errorf("deliveryFee = %+v", order.Delivery.DeliveryFee)
	}

	if len(order.Payments.Methods) != 1 || order.Payments.Methods[0].Method != PaymentPix {
		t.Errorf("payments = %+v", order.Payments)
	}
	if order.Payments.Prepaid != 38.89 || order.Payments.Pending != 0 {
		t.Errorf("payments = %+v", order.Payments)
	}
}

// Brendi sends "eventId" in polling, but ack requires "id".
func TestEventAckMapping(t *testing.T) {
	var events []Event
	if err := json.Unmarshal(readFixture(t, "events.json"), &events); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}

	event := events[0]
	if event.Key() != "87d0384a-a52a-4d6f-9523-448a22d99831" {
		t.Errorf("key = %q", event.Key())
	}
	if event.EventType != EventDelivered || event.OrderID == "" {
		t.Errorf("event = %+v", event)
	}
	if !event.CreatedAt.Valid() {
		t.Errorf("createdAt did not parse: %q", event.CreatedAt.Raw)
	}

	ack := event.Ack()
	if ack.ID != event.Key() || ack.OrderID != event.OrderID || ack.EventType != event.EventType {
		t.Errorf("ack = %+v", ack)
	}

	// The ack must be emitted exactly with the keys accepted by Brendi.
	data, err := json.Marshal([]AckEvent{ack})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `[{"id":"87d0384a-a52a-4d6f-9523-448a22d99831","orderId":"ababf637-6a62-4922-8c1a-4b774072d930","eventType":"DELIVERED"}]`
	if string(data) != want {
		t.Errorf("ack json = %s", data)
	}
}

func TestDateTimeToleratesUnknownFormat(t *testing.T) {
	var d DateTime
	if err := json.Unmarshal([]byte(`"04/11/2026 20:00"`), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Valid() {
		t.Errorf("expected an invalid date, got %v", d.Time)
	}
	if d.String() != "04/11/2026 20:00" {
		t.Errorf("raw value lost: %q", d.String())
	}
}
