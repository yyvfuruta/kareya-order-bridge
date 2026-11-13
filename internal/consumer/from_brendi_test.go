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
		t.Fatalf("read fixture: %v", err)
	}
	var order brendi.Order
	if err := json.Unmarshal(raw, &order); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return &order
}

func TestFromBrendi(t *testing.T) {
	order := loadOrder(t)
	got := FromBrendi(order, ProductMap{
		ByName: map[string]string{"Donburi katsudon": "112"},
	})

	if got.ID != order.ID {
		t.Errorf("id = %q, want %q", got.ID, order.ID)
	}
	if got.DisplayID != "661009" {
		t.Errorf("displayId = %q", got.DisplayID)
	}
	if got.OrderTiming != "IMMEDIATE" {
		t.Errorf("orderTiming = %q, want IMMEDIATE", got.OrderTiming)
	}
	if got.CreatedAt == "" || got.PreparationStartDateTime == "" {
		t.Errorf("empty dates: createdAt=%q preparation=%q", got.CreatedAt, got.PreparationStartDateTime)
	}

	if len(got.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(got.Items))
	}
	item := got.Items[0]
	if item.Name != "Donburi katsudon" {
		t.Errorf("item name = %q", item.Name)
	}
	if item.ExternalCode != "112" {
		t.Errorf("externalCode = %q, want the name mapping", item.ExternalCode)
	}
	if item.Quantity != 1 || item.UnitPrice != 37.03 || item.TotalPrice != 37.03 {
		t.Errorf("wrong item amounts: %+v", item)
	}
	if item.ID == "" {
		t.Error("item without id")
	}

	if got.Total.SubTotal != 37.03 {
		t.Errorf("subTotal = %v", got.Total.SubTotal)
	}
	if got.Total.DeliveryFee != 4.49 {
		t.Errorf("deliveryFee = %v, want 4.49", got.Total.DeliveryFee)
	}
	if got.Total.AdditionalFees != 1.86 {
		t.Errorf("additionalFees = %v, want 1.86 (6.35 - 4.49)", got.Total.AdditionalFees)
	}
	if got.Total.Benefits != 4.49 {
		t.Errorf("benefits = %v, want 4.49", got.Total.Benefits)
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
		t.Errorf("wrong address: %+v", addr)
	}
	if addr.City != "Curitiba" || addr.State != "PR" || addr.PostalCode != "80000-000" {
		t.Errorf("wrong city/state/postal code: %+v", addr)
	}
	if addr.Coordinates.Latitude != -25.4 || addr.Coordinates.Longitude != -49.2 {
		t.Errorf("wrong coordinates: %+v", addr.Coordinates)
	}

	if got.Customer == nil || got.Customer.Name != "Cliente Teste" {
		t.Errorf("wrong customer: %+v", got.Customer)
	}
	if got.Customer != nil && got.Customer.Phone.Number != "5511999999999" {
		t.Errorf("phone = %q", got.Customer.Phone.Number)
	}

	if got.Payments.Prepaid != 38.89 || got.Payments.Pending != 0 {
		t.Errorf("wrong payments: %+v", got.Payments)
	}
	if len(got.Payments.Methods) != 1 || got.Payments.Methods[0].Method != "PIX" {
		t.Errorf("wrong payment methods: %+v", got.Payments.Methods)
	}
}

func TestProductMapPrefersExternalCode(t *testing.T) {
	products := ProductMap{
		ByExternalCode: map[string]string{"ABC": "999"},
		ByName:         map[string]string{"ABC": "111"},
	}
	if got := products.Lookup("ABC", "ABC"); got != "999" {
		t.Errorf("lookup = %q, want 999", got)
	}
	if got := products.Lookup("", "ABC"); got != "111" {
		t.Errorf("name lookup = %q, want 111", got)
	}
	if got := products.Lookup("", "NO MAP"); got != "" {
		t.Errorf("unmapped lookup = %q, want empty", got)
	}
}

func TestCustomerIDFallback(t *testing.T) {
	for _, sourceID := range []string{"", "  ", "customer-from-source"} {
		t.Run(sourceID, func(t *testing.T) {
			order := loadOrder(t)
			order.Customer.ID = sourceID
			got := FromBrendi(order, ProductMap{})
			if got.Customer.ID == "" || got.Customer.ID == "  " {
				t.Fatal("customer id is empty")
			}
			if sourceID == "customer-from-source" && got.Customer.ID != sourceID {
				t.Fatal("source customer id was replaced")
			}
			if got.Customer.ID != FromBrendi(order, ProductMap{}).Customer.ID {
				t.Fatal("customer id changed on retry")
			}
			if got.Customer.Name != order.Customer.Name || got.Customer.Phone.Number != order.Customer.Phone.Number {
				t.Fatal("customer details were lost")
			}
			if order.Customer.ID != sourceID {
				t.Fatal("source customer was mutated")
			}
		})
	}
}

func TestConfiguredFreeOptionsBecomeObservations(t *testing.T) {
	order := loadOrder(t)
	order.Items[0].SpecialInstructions = "picancia forte. hashi"
	order.Items[0].Options = []brendi.ItemOption{
		{Name: "chicken - peito de frango empanado", Quantity: 1},
		{Name: "Enviar hashi", Quantity: 2, SpecialInstructions: "embalado"},
	}
	products := ProductMap{OptionsAsObservations: []string{"Enviar hashi", "chicken - peito de frango empanado"}}
	got := FromBrendi(order, products)
	if len(got.Items[0].Options) != 0 {
		t.Fatal("configured free instructions still appear as inventory options")
	}
	want := "picancia forte. hashi\n1x chicken - peito de frango empanado\n2x Enviar hashi (embalado)"
	if got.Items[0].Observations != want {
		t.Fatalf("observations = %q, want %q", got.Items[0].Observations, want)
	}
	if got.Total != FromBrendi(order, ProductMap{}).Total || got.Items[0].TotalPrice != order.Items[0].TotalPrice.Value {
		t.Fatal("converting instructions changed amounts")
	}
	if len(order.Items[0].Options) != 2 {
		t.Fatal("source options were mutated")
	}
}

func TestObservationConversionPreservesPaidMappedAndUnknownOptions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		option   brendi.ItemOption
		products ProductMap
	}{
		{"paid unit", brendi.ItemOption{Name: "Enviar hashi", Quantity: 1, UnitPrice: brendi.Price{Value: 2}}, ProductMap{}},
		{"paid total", brendi.ItemOption{Name: "Enviar hashi", Quantity: 1, TotalPrice: brendi.Price{Value: 2}}, ProductMap{}},
		{"source code", brendi.ItemOption{Name: "Enviar hashi", Quantity: 1, ExternalCode: "123"}, ProductMap{}},
		{"mapped code", brendi.ItemOption{Name: "Enviar hashi", Quantity: 1}, ProductMap{ByName: map[string]string{"Enviar hashi": "123"}}},
		{"unknown name", brendi.ItemOption{Name: "Outro complemento", Quantity: 1}, ProductMap{}},
		{"invalid quantity", brendi.ItemOption{Name: "Enviar hashi", Quantity: 0}, ProductMap{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := loadOrder(t)
			order.Items[0].Options = []brendi.ItemOption{tc.option}
			tc.products.OptionsAsObservations = []string{"Enviar hashi"}
			got := FromBrendi(order, tc.products).Items[0]
			if len(got.Options) != 1 || got.Options[0].ID == "" || got.Options[0].Name != tc.option.Name {
				t.Fatalf("option was lost: %+v", got)
			}
			if got.Observations != order.Items[0].SpecialInstructions {
				t.Fatal("unexpected observation conversion")
			}
		})
	}
}

func TestRepeatedItemIndexes(t *testing.T) {
	order := loadOrder(t)
	order.Items[0].Index = 1
	second := order.Items[0]
	second.ID = "second-item"
	order.Items = append(order.Items, second)
	got := FromBrendi(order, ProductMap{})
	if got.Items[0].Index != 1 || got.Items[1].Index != 2 {
		t.Fatalf("indexes = %d, %d", got.Items[0].Index, got.Items[1].Index)
	}
	if got.Items[1].ID != second.ID || order.Items[1].Index != 1 {
		t.Fatal("source item or its id changed")
	}
}

func TestOptionIDsInJSON(t *testing.T) {
	order := loadOrder(t)
	sourceID := "67a3158b-54ca-4f88-b69a-bef78b82ac70"
	order.Items[0].Options = []brendi.ItemOption{
		{ID: sourceID, Name: "Molho", Quantity: 1},
		{Name: "Enviar hashi", Quantity: 1},
		{Name: "Enviar hashi", Quantity: 1},
	}
	secondItem := order.Items[0]
	order.Items = append(order.Items, secondItem)

	first := FromBrendi(order, ProductMap{})
	retry := FromBrendi(order, ProductMap{})
	if first.Items[0].Options[0].ID != sourceID {
		t.Fatal("source option id was not preserved")
	}
	seen := map[string]bool{}
	for i, item := range first.Items {
		for j, option := range item.Options {
			if option.ID == "" {
				t.Fatalf("item %d option %d has no id", i, j)
			}
			if option.ID != retry.Items[i].Options[j].ID {
				t.Fatalf("item %d option %d id changed on retry", i, j)
			}
			if j > 0 {
				if seen[option.ID] {
					t.Fatalf("generated option id is not unique: %s", option.ID)
				}
				seen[option.ID] = true
			}
		}
	}
	raw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Items []struct {
			Options []map[string]any `json:"options"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	for i, item := range payload.Items {
		for j, option := range item.Options {
			if id, ok := option["id"].(string); !ok || id == "" {
				t.Fatalf("item %d option %d JSON lacks a nonempty string id", i, j)
			}
		}
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
		t.Errorf("card.brand = %q, want VISA", card.Brand)
	}
}

func TestFromBrendiPaymentTypesAndJSONStructure(t *testing.T) {
	order := loadOrder(t)
	order.Payments.Methods = []brendi.Payment{
		{
			Value:    20.0,
			Currency: "BRL",
			Type:     brendi.PaymentTypePrepaid,
			Method:   "CREDIT",
		},
		{
			Value:     18.89,
			Currency:  "BRL",
			Type:      brendi.PaymentTypePending,
			Method:    "CASH",
			ChangeFor: 50.0,
		},
	}

	got := FromBrendi(order, ProductMap{})

	if len(got.Payments.Methods) != 2 {
		t.Fatalf("methods = %d, want 2", len(got.Payments.Methods))
	}
	if got.Payments.Methods[0].Type != "ONLINE" || !got.Payments.Methods[0].Prepaid {
		t.Errorf("method[0]: type=%q prepaid=%v, want ONLINE/true", got.Payments.Methods[0].Type, got.Payments.Methods[0].Prepaid)
	}
	if got.Payments.Methods[1].Type != "OFFLINE" || got.Payments.Methods[1].Prepaid {
		t.Errorf("method[1]: type=%q prepaid=%v, want OFFLINE/false", got.Payments.Methods[1].Type, got.Payments.Methods[1].Prepaid)
	}
	if got.Payments.Methods[1].Cash == nil || got.Payments.Methods[1].Cash.ChangeFor != 50.0 {
		t.Errorf("cash changeFor = %+v", got.Payments.Methods[1].Cash)
	}

	// Verify JSON output
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	totalMap, ok := data["total"].(map[string]any)
	if !ok {
		t.Fatalf("total not a map: %v", data["total"])
	}
	if _, ok := totalMap["subTotal"]; !ok {
		t.Errorf("subTotal key missing in total: %+v", totalMap)
	}
	if _, hasBad := totalMap["sub Total"]; hasBad {
		t.Errorf("bad 'sub Total' key found in total: %+v", totalMap)
	}

	custMap, ok := data["customer"].(map[string]any)
	if !ok {
		t.Fatalf("customer not a map: %v", data["customer"])
	}
	phoneMap, ok := custMap["phone"].(map[string]any)
	if !ok {
		t.Fatalf("phone not a map: %v", custMap["phone"])
	}
	if v, exists := phoneMap["localizer"]; exists && v == "" {
		t.Errorf("localizer must not be empty string: %v", v)
	}
	if v, exists := phoneMap["localizerExpiration"]; exists && v == "" {
		t.Errorf("localizerExpiration must not be empty string: %v", v)
	}
}
