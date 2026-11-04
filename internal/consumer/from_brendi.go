package consumer

import (
	"crypto/sha1"
	"fmt"
	"strings"

	"kareya-order-bridge/internal/brendi"
)

// ProductMap traduz o codigo/nome do produto na Brendi para o codigo PDV
// cadastrado no Consumer. A Brendi as vezes manda externalCode vazio (pedidos
// de marketplace), por isso o fallback por nome.
type ProductMap struct {
	ByExternalCode map[string]string `json:"byExternalCode"`
	ByName         map[string]string `json:"byName"`
}

// Lookup devolve o codigo PDV do produto.
func (m ProductMap) Lookup(externalCode, name string) string {
	if code := m.ByExternalCode[externalCode]; code != "" {
		return code
	}
	if code := m.ByName[name]; code != "" {
		return code
	}
	return externalCode
}

// FromBrendi converte o pedido da Brendi para o formato do Consumer.
func FromBrendi(o *brendi.Order, products ProductMap) Order {
	out := Order{
		ID:                       o.ID,
		DisplayID:                o.DisplayID,
		OrderType:                string(o.Type),
		SalesChannel:             o.SalesChannel,
		OrderTiming:              timing(o.OrderTiming),
		CreatedAt:                o.CreatedAt.String(),
		PreparationStartDateTime: o.PreparationStartDateTime.String(),
		Merchant:                 Merchant{ID: o.Merchant.ID, Name: o.Merchant.Name},
		Items:                    items(o, products),
		Benefits:                 benefits(o.Discounts),
		Total:                    total(o),
		Payments:                 payments(o.Payments),
		ExtraInfo:                o.ExtraInfo,
	}

	if o.Customer != nil {
		out.Customer = &Customer{
			ID:                    o.Customer.ID,
			Name:                  o.Customer.Name,
			DocumentNumber:        o.Customer.DocumentNumber,
			Email:                 o.Customer.Email,
			OrdersCountOnMerchant: o.Customer.OrdersCountOnMerchant,
			Phone: Phone{
				Number:    o.Customer.Phone.Number,
				Extension: o.Customer.Phone.Extension,
			},
		}
	}

	if o.Delivery != nil {
		out.Delivery = &Delivery{
			Mode:             "DEFAULT",
			DeliveredBy:      string(o.Delivery.DeliveredBy),
			PickupCode:       o.Delivery.PickupCode,
			DeliveryDateTime: firstValid(o.Delivery.DeliveryDateTime, o.Delivery.EstimatedDeliveryDateTime),
			DeliveryAddress: Address{
				Country:          o.Delivery.DeliveryAddress.Country,
				State:            o.Delivery.DeliveryAddress.State,
				City:             o.Delivery.DeliveryAddress.City,
				PostalCode:       o.Delivery.DeliveryAddress.PostalCode,
				StreetName:       o.Delivery.DeliveryAddress.Street,
				StreetNumber:     o.Delivery.DeliveryAddress.Number,
				Neighborhood:     o.Delivery.DeliveryAddress.District,
				Complement:       o.Delivery.DeliveryAddress.Complement,
				Reference:        o.Delivery.DeliveryAddress.Reference,
				FormattedAddress: o.Delivery.DeliveryAddress.FormattedAddress,
				Coordinates: Coordinates{
					Latitude:  o.Delivery.DeliveryAddress.Coordinates.Latitude,
					Longitude: o.Delivery.DeliveryAddress.Coordinates.Longitude,
				},
			},
		}
	}

	if o.Takeout != nil {
		out.Takeout = &Takeout{
			Mode:            string(o.Takeout.Mode),
			TakeoutDateTime: o.Takeout.TakeoutDateTime.String(),
		}
	}

	if o.Indoor != nil {
		out.Indoor = &Indoor{
			Mode:           string(o.Indoor.Mode),
			IndoorDateTime: o.Indoor.IndoorDateTime.String(),
			Place:          o.Indoor.Place,
			Tab:            o.Indoor.Tab,
			Seat:           o.Indoor.Seat,
			WaiterCode:     o.Indoor.WaiterCode,
		}
	}

	return out
}

func timing(t brendi.OrderTiming) string {
	switch t {
	case brendi.OrderTimingInstant:
		return "IMMEDIATE"
	case brendi.OrderTimingScheduled:
		return "SCHEDULED"
	case brendi.OrderTimingOnDemand:
		return "ONDEMAND"
	default:
		return string(t)
	}
}

func items(o *brendi.Order, products ProductMap) []Item {
	out := make([]Item, 0, len(o.Items))
	for i, item := range o.Items {
		id := item.ID
		if id == "" {
			id = deterministicID(o.ID, "item", fmt.Sprint(i))
		}

		options := make([]Option, 0, len(item.Options))
		for j, option := range item.Options {
			options = append(options, Option{
				Name:         option.Name,
				ExternalCode: products.Lookup(option.ExternalCode, option.Name),
				Quantity:     option.Quantity,
				UnitPrice:    option.UnitPrice.Value,
				Price:        option.TotalPrice.Value,
				Index:        j,
			})
		}

		out = append(out, Item{
			ID:           id,
			Index:        item.Index,
			Name:         item.Name,
			ExternalCode: products.Lookup(item.ExternalCode, item.Name),
			Unit:         string(item.Unit),
			Quantity:     item.Quantity,
			UnitPrice:    item.UnitPrice.Value,
			TotalPrice:   item.TotalPrice.Value,
			Observations: item.SpecialInstructions,
			Options:      options,
		})
	}
	return out
}

func total(o *brendi.Order) Total {
	fee := deliveryFee(o)
	return Total{
		SubTotal:       o.Total.ItemsPrice.Value,
		DeliveryFee:    round2(fee),
		AdditionalFees: round2(o.Total.OtherFees.Value - fee),
		Benefits:       round2(o.Total.Discount.Value),
		OrderAmount:    o.Total.OrderAmount.Value,
	}
}

func deliveryFee(o *brendi.Order) float64 {
	if o.Delivery != nil && o.Delivery.DeliveryFee != nil {
		return o.Delivery.DeliveryFee.Value
	}
	var sum float64
	for _, fee := range o.OtherFees {
		if fee.Type == brendi.FeeDelivery {
			sum += fee.Price.Value
		}
	}
	return sum
}

func payments(p brendi.Payments) Payments {
	out := Payments{Prepaid: p.Prepaid, Pending: p.Pending, Methods: make([]Payment, 0, len(p.Methods))}
	for _, m := range p.Methods {
		payment := Payment{
			Value:      m.Value,
			Currency:   m.Currency,
			Type:       string(m.Type),
			Method:     string(m.Method),
			Brand:      string(m.Brand),
			MethodInfo: m.MethodInfo,
			ChangeFor:  m.ChangeFor,
		}
		if m.Transaction != nil {
			payment.Transaction = &Transaction{
				AuthorizationCode: m.Transaction.AuthorizationCode,
				AcquirerDocument:  m.Transaction.AcquirerDocument,
			}
		}
		if m.Brand != "" {
			payment.Card = &Card{Brand: string(m.Brand)}
		}
		out.Methods = append(out.Methods, payment)
	}
	return out
}

func benefits(discounts []brendi.Discount) []Benefit {
	out := make([]Benefit, 0, len(discounts))
	for _, d := range discounts {
		b := Benefit{
			Target:   string(d.Target),
			TargetID: d.TargetID,
			Value:    d.Amount.Value,
		}
		for _, s := range d.SponsorshipValues {
			b.SponsorshipValues = append(b.SponsorshipValues, SponsorshipValue{
				Name:        string(s.Name),
				Value:       s.Amount.Value,
				Description: s.DiscountCode,
			})
		}
		out = append(out, b)
	}
	return out
}

func firstValid(values ...brendi.DateTime) string {
	for _, v := range values {
		if s := v.String(); s != "" {
			return s
		}
	}
	return ""
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// deterministicID gera um UUID estavel para itens sem id na origem.
func deterministicID(namespace string, parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(append([]string{namespace}, parts...), "\x00")))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
