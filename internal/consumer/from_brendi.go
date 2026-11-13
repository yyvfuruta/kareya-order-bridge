package consumer

import (
	"crypto/sha1"
	"fmt"
	"slices"
	"strings"
	"time"

	"kareya-order-bridge/internal/brendi"
)

// ProductMap translates the Brendi product code/name into the PDV code
// registered in Consumer. Brendi sometimes sends an empty externalCode
// (marketplace orders), hence the name fallback.
type ProductMap struct {
	ByExternalCode        map[string]string     `json:"byExternalCode"`
	ByName                map[string]string     `json:"byName"`
	OptionsAsObservations []string              `json:"optionsAsObservations,omitempty"`
	BySalesChannel        map[string]ProductMap `json:"bySalesChannel,omitempty"`
}

// Lookup returns the product PDV code.
func (m ProductMap) Lookup(externalCode, name string) string {
	if code := m.ByExternalCode[externalCode]; code != "" {
		return code
	}
	if code := m.ByName[name]; code != "" {
		return code
	}
	return externalCode
}

// FromBrendi converts a Brendi order into the Consumer format.
func FromBrendi(o *brendi.Order, products ProductMap) Order {
	// A channel-specific map replaces the default map only for that channel.
	if channelMap, ok := products.BySalesChannel[o.SalesChannel]; ok {
		products = channelMap
	}
	salesChannel := o.SalesChannel
	if salesChannel == "" {
		salesChannel = "PARTNER"
	}

	createdAt := o.CreatedAt.ISO8601()
	if createdAt == "" {
		createdAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	}

	prepStart := o.PreparationStartDateTime.ISO8601()
	if prepStart == "" {
		prepStart = createdAt
	}

	out := Order{
		ID:                       o.ID,
		DisplayID:                o.DisplayID,
		OrderType:                string(o.Type),
		SalesChannel:             salesChannel,
		OrderTiming:              timing(o.OrderTiming),
		CreatedAt:                createdAt,
		PreparationStartDateTime: prepStart,
		Merchant:                 Merchant{ID: o.Merchant.ID, Name: o.Merchant.Name},
		Items:                    items(o, products),
		Benefits:                 benefits(o.Discounts),
		Total:                    total(o),
		Payments:                 payments(o.Payments),
		ExtraInfo:                o.ExtraInfo,
	}

	if o.Customer != nil {
		cust := &Customer{
			ID:   o.Customer.ID,
			Name: o.Customer.Name,
			Phone: Phone{
				Number:    o.Customer.Phone.Number,
				Extension: o.Customer.Phone.Extension,
			},
		}
		if strings.TrimSpace(cust.ID) == "" {
			cust.ID = deterministicID(o.ID, "customer")
		}
		if o.Customer.DocumentNumber != "" {
			cust.DocumentNumber = &o.Customer.DocumentNumber
		}
		if o.Customer.OrdersCountOnMerchant > 0 {
			cust.OrdersCountOnMerchant = &o.Customer.OrdersCountOnMerchant
		}
		if o.Customer.Phone.Localizer != "" {
			cust.Phone.Localizer = &o.Customer.Phone.Localizer
		}
		if exp := o.Customer.Phone.LocalizerExpiration.ISO8601(); exp != "" {
			cust.Phone.LocalizerExpiration = &exp
		}
		out.Customer = cust
	} else {
		out.Customer = &Customer{
			ID:   deterministicID(o.ID, "customer"),
			Name: "Cliente",
			Phone: Phone{
				Number: "N/D",
			},
		}
	}

	if o.Delivery != nil {
		country := o.Delivery.DeliveryAddress.Country
		if country == "" {
			country = "BR"
		}
		pickupCode := o.Delivery.PickupCode
		if pickupCode == "" {
			pickupCode = "N/D"
		}
		deliveryDT := firstValid(o.Delivery.DeliveryDateTime, o.Delivery.EstimatedDeliveryDateTime)
		if deliveryDT == "" {
			deliveryDT = createdAt
		}

		out.Delivery = &Delivery{
			Mode:             "DEFAULT",
			DeliveredBy:      string(o.Delivery.DeliveredBy),
			PickupCode:       pickupCode,
			DeliveryDateTime: deliveryDT,
			DeliveryAddress: Address{
				Country:          country,
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
			TakeoutDateTime: o.Takeout.TakeoutDateTime.ISO8601(),
		}
	}

	if o.Indoor != nil {
		out.Indoor = &Indoor{
			Mode:           string(o.Indoor.Mode),
			IndoorDateTime: o.Indoor.IndoorDateTime.ISO8601(),
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

		observations := item.SpecialInstructions
		options := make([]Option, 0, len(item.Options))
		for j, option := range item.Options {
			externalCode := products.Lookup(option.ExternalCode, option.Name)
			if externalCode == "" && option.UnitPrice.Value == 0 && option.TotalPrice.Value == 0 &&
				option.Quantity > 0 && slices.Contains(products.OptionsAsObservations, option.Name) {
				text := fmt.Sprintf("%gx %s", option.Quantity, option.Name)
				if option.SpecialInstructions != "" {
					text += " (" + option.SpecialInstructions + ")"
				}
				if observations != "" {
					observations += "\n"
				}
				observations += text
				continue
			}
			optionID := option.ID
			if optionID == "" {
				optionID = deterministicID(o.ID, "item", fmt.Sprint(i), "option", fmt.Sprint(j))
			}
			options = append(options, Option{
				ID:           optionID,
				Name:         option.Name,
				ExternalCode: externalCode,
				Quantity:     option.Quantity,
				UnitPrice:    option.UnitPrice.Value,
				Price:        option.TotalPrice.Value,
				Index:        j + 1,
			})
		}

		unitPrice := item.UnitPrice.Value
		totalPrice := item.TotalPrice.Value
		if totalPrice == 0 && unitPrice > 0 {
			totalPrice = unitPrice * item.Quantity
		}

		unit := string(item.Unit)
		if unit == "" {
			unit = "UN"
		}

		out = append(out, Item{
			ID:           id,
			Index:        i + 1,
			Name:         item.Name,
			ExternalCode: products.Lookup(item.ExternalCode, item.Name),
			Unit:         unit,
			Quantity:     item.Quantity,
			UnitPrice:    unitPrice,
			TotalPrice:   totalPrice,
			Price:        totalPrice,
			Observations: observations,
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
		pType := paymentType(m.Type, m.Prepaid, p.Prepaid > 0)
		isPrepaid := pType == "ONLINE"
		currency := m.Currency
		if currency == "" {
			currency = "BRL"
		}
		payment := Payment{
			Value:      m.Value,
			Currency:   currency,
			Type:       pType,
			Prepaid:    isPrepaid,
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
		if m.ChangeFor > 0 {
			payment.Cash = &Cash{ChangeFor: m.ChangeFor}
		}
		out.Methods = append(out.Methods, payment)
	}
	return out
}

func paymentType(t brendi.PaymentType, methodPrepaid bool, orderHasPrepaid bool) string {
	switch strings.ToUpper(string(t)) {
	case "ONLINE", "PREPAID":
		return "ONLINE"
	case "OFFLINE", "PENDING":
		return "OFFLINE"
	default:
		if methodPrepaid || orderHasPrepaid {
			return "ONLINE"
		}
		return "OFFLINE"
	}
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
		if s := v.ISO8601(); s != "" {
			return s
		}
	}
	return ""
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// deterministicID builds a stable UUID for records without an id at the source.
func deterministicID(namespace string, parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(append([]string{namespace}, parts...), "\x00")))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
