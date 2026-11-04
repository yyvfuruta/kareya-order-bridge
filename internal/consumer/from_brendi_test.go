package consumer

import (
	"encoding/json"
	"os"
	"testing"

	"kareya-order-bridge/internal/brendi"
)

func loadOrder(t *testing.T) *brendi.Order {
	t.Helper()
	raw, err := os.ReadFile("../brendi/testdata/order.json")
	if err != nil {
		t.Fatalf("ler fixture: %v", err)
	}
	var order brendi.Order
	if err := json.Unmarshal(raw, &order); err != nil {
		t.Fatalf("decodificar fixture: %v", err)
	}
	return &order
}

func TestFromBrendi(t *testing.T) {
	order := loadOrder(t)
	got := FromBrendi(order, ProductMap{
		ByName: map[string]string{"Donburi katsudon": "112"},
	})

	if got.ID != order.ID {
		t.Errorf("id = %q, quer %q", got.ID, order.ID)
	}
	if got.DisplayID != "661009" {
		t.Errorf("displayId = %q", got.DisplayID)
	}
	if got.OrderTiming != "IMMEDIATE" {
		t.Errorf("orderTiming = %q, quer IMMEDIATE", got.OrderTiming)
	}
	if got.CreatedAt == "" || got.PreparationStartDateTime == "" {
		t.Errorf("datas vazias: createdAt=%q preparation=%q", got.CreatedAt, got.PreparationStartDateTime)
	}

	if len(got.Items) != 1 {
		t.Fatalf("itens = %d, quer 1", len(got.Items))
	}
	item := got.Items[0]
	if item.Name != "Donburi katsudon" {
		t.Errorf("nome do item = %q", item.Name)
	}
	if item.ExternalCode != "112" {
		t.Errorf("externalCode = %q, quer o de-para por nome", item.ExternalCode)
	}
	if item.Quantity != 1 || item.UnitPrice != 37.03 || item.TotalPrice != 37.03 {
		t.Errorf("valores do item errados: %+v", item)
	}
	if item.ID == "" {
		t.Error("item sem id")
	}

	if got.Total.SubTotal != 37.03 {
		t.Errorf("subTotal = %v", got.Total.SubTotal)
	}
	if got.Total.DeliveryFee != 4.49 {
		t.Errorf("deliveryFee = %v, quer 4.49", got.Total.DeliveryFee)
	}
	if got.Total.AdditionalFees != 1.86 {
		t.Errorf("additionalFees = %v, quer 1.86 (6.35 - 4.49)", got.Total.AdditionalFees)
	}
	if got.Total.Benefits != 4.49 {
		t.Errorf("benefits = %v, quer 4.49", got.Total.Benefits)
	}
	if got.Total.OrderAmount != 38.89 {
		t.Errorf("orderAmount = %v", got.Total.OrderAmount)
	}

	if got.Delivery == nil {
		t.Fatal("delivery nil")
	}
	if got.Delivery.Mode != "DEFAULT" {
		t.Errorf("delivery.mode = %q", got.Delivery.Mode)
	}
	if got.Delivery.DeliveredBy != "MERCHANT" {
		t.Errorf("deliveredBy = %q", got.Delivery.DeliveredBy)
	}
	addr := got.Delivery.DeliveryAddress
	if addr.StreetName != "Rua Teste" || addr.StreetNumber != "123" || addr.Neighborhood != "Centro" {
		t.Errorf("endereco errado: %+v", addr)
	}
	if addr.City != "Curitiba" || addr.State != "PR" || addr.PostalCode != "80000-000" {
		t.Errorf("cidade/uf/cep errados: %+v", addr)
	}
	if addr.Coordinates.Latitude != -25.4 || addr.Coordinates.Longitude != -49.2 {
		t.Errorf("coordenadas erradas: %+v", addr.Coordinates)
	}

	if got.Customer == nil || got.Customer.Name != "Cliente Teste" {
		t.Errorf("cliente errado: %+v", got.Customer)
	}
	if got.Customer != nil && got.Customer.Phone.Number != "5511999999999" {
		t.Errorf("telefone = %q", got.Customer.Phone.Number)
	}

	if got.Payments.Prepaid != 38.89 || got.Payments.Pending != 0 {
		t.Errorf("payments errados: %+v", got.Payments)
	}
	if len(got.Payments.Methods) != 1 || got.Payments.Methods[0].Method != "PIX" {
		t.Errorf("formas de pagamento erradas: %+v", got.Payments.Methods)
	}
}

func TestProductMapPrefersExternalCode(t *testing.T) {
	products := ProductMap{
		ByExternalCode: map[string]string{"ABC": "999"},
		ByName:         map[string]string{"ABC": "111"},
	}
	if got := products.Lookup("ABC", "ABC"); got != "999" {
		t.Errorf("lookup = %q, quer 999", got)
	}
	if got := products.Lookup("", "ABC"); got != "111" {
		t.Errorf("lookup por nome = %q, quer 111", got)
	}
	if got := products.Lookup("", "SEM MAPA"); got != "" {
		t.Errorf("lookup sem mapa = %q, quer vazio", got)
	}
}

func TestPaymentCardBrand(t *testing.T) {
	order := loadOrder(t)
	order.Payments.Methods[0].Brand = "VISA"

	got := FromBrendi(order, ProductMap{})
	card := got.Payments.Methods[0].Card
	if card == nil {
		t.Fatal("card nil")
	}
	if card.Brand != "VISA" {
		t.Errorf("card.brand = %q, quer VISA", card.Brand)
	}
}
