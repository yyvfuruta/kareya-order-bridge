// Package consumer defines the partner API schemas expected by Consumer (the
// restaurant management system) and the translation from Brendi orders.
package consumer

// PollingResponse is the body returned by the polling endpoint.
type PollingResponse struct {
	Items        []PollingEvent `json:"items"`
	StatusCode   int            `json:"statusCode"`
	ReasonPhrase *string        `json:"reasonPhrase"`
}

// PollingEvent is a new-order event.
type PollingEvent struct {
	ID         string `json:"id"`
	OrderID    string `json:"orderId"`
	CreatedAt  string `json:"createdAt"`
	FullCode   string `json:"fullCode"`
	Code       string `json:"code"`
	StatusCode int    `json:"statusCode"`
}

// OrderResponse is the body returned by the order details endpoint.
type OrderResponse struct {
	Item         Order   `json:"item"`
	StatusCode   int     `json:"statusCode"`
	ReasonPhrase *string `json:"reasonPhrase"`
}

// EventResponse is the body returned by the single-event endpoint.
type EventResponse struct {
	Item         PollingEvent `json:"item"`
	StatusCode   int          `json:"statusCode"`
	ReasonPhrase *string      `json:"reasonPhrase"`
}

// StoresResponse is the body returned by the store listing endpoint.
type StoresResponse struct {
	Items        []Store `json:"items"`
	StatusCode   int     `json:"statusCode"`
	ReasonPhrase *string `json:"reasonPhrase"`
}

// Store identifies a partner store.
type Store struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// StatusRequest is the body sent by Consumer when changing an order status.
type StatusRequest struct {
	OrderID       string `json:"orderId"`
	Status        string `json:"status"`
	Justification string `json:"justification,omitempty"`
}

// Response is the standard success/error response for operations.
type Response struct {
	StatusCode   int     `json:"statusCode"`
	ReasonPhrase *string `json:"reasonPhrase"`
}

// Order is the order in the format expected by Consumer.
type Order struct {
	ID                       string    `json:"id"`
	DisplayID                string    `json:"displayId"`
	OrderType                string    `json:"orderType"`
	SalesChannel             string    `json:"salesChannel"`
	OrderTiming              string    `json:"orderTiming"`
	CreatedAt                string    `json:"createdAt"`
	PreparationStartDateTime string    `json:"preparationStartDateTime"`
	Merchant                 Merchant  `json:"merchant"`
	Items                    []Item    `json:"items"`
	Benefits                 []Benefit `json:"benefits,omitempty"`
	Total                    Total     `json:"total"`
	Payments                 Payments  `json:"payments"`
	Customer                 *Customer `json:"customer,omitempty"`
	Delivery                 *Delivery `json:"delivery,omitempty"`
	Takeout                  *Takeout  `json:"takeout,omitempty"`
	Indoor                   *Indoor   `json:"indoor,omitempty"`
	ExtraInfo                string    `json:"extraInfo,omitempty"`
}

// Merchant is the store that received the order.
type Merchant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Item is an order line.
type Item struct {
	ID               string   `json:"id"`
	Index            int      `json:"index,omitempty"`
	Name             string   `json:"name"`
	ExternalCode     string   `json:"externalCode"`
	Unit             string   `json:"unit,omitempty"`
	Quantity         float64  `json:"quantity"`
	UnitPrice        float64  `json:"unitPrice"`
	TotalPrice       float64  `json:"totalPrice"`
	Price            float64  `json:"price,omitempty"`
	Observations     string   `json:"observations,omitempty"`
	ImageURL         string   `json:"imageUrl,omitempty"`
	Options          []Option `json:"options,omitempty"`
	ConsumerItemInfo any      `json:"consumerItemInfo,omitempty"`
}

// Option is an item modifier.
type Option struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	ExternalCode string  `json:"externalCode,omitempty"`
	Quantity     float64 `json:"quantity"`
	UnitPrice    float64 `json:"unitPrice"`
	Price        float64 `json:"price"`
	Index        int     `json:"index,omitempty"`
}

// Benefit is a discount applied to the order.
type Benefit struct {
	Target            string             `json:"target"`
	TargetID          string             `json:"targetId,omitempty"`
	Value             float64            `json:"value"`
	SponsorshipValues []SponsorshipValue `json:"sponsorshipValues,omitempty"`
}

// SponsorshipValue says who funded the discount.
type SponsorshipValue struct {
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	Description string  `json:"description,omitempty"`
}

// Total summarizes the order amounts.
type Total struct {
	SubTotal       float64 `json:"subTotal"`
	DeliveryFee    float64 `json:"deliveryFee"`
	Benefits       float64 `json:"benefits"`
	AdditionalFees float64 `json:"additionalFees"`
	OrderAmount    float64 `json:"orderAmount"`
}

// Payments describes the order payment.
type Payments struct {
	Prepaid float64   `json:"prepaid"`
	Pending float64   `json:"pending"`
	Methods []Payment `json:"methods"`
}

// Payment is a payment method.
type Payment struct {
	Value       float64      `json:"value"`
	Currency    string       `json:"currency"`
	Type        string       `json:"type"`
	Method      string       `json:"method"`
	Prepaid     bool         `json:"prepaid"`
	Brand       string       `json:"brand,omitempty"`
	MethodInfo  string       `json:"methodInfo,omitempty"`
	Card        *Card        `json:"card,omitempty"`
	Cash        *Cash        `json:"cash,omitempty"`
	Transaction *Transaction `json:"transaction,omitempty"`
	ChangeFor   float64      `json:"changeFor,omitempty"`
}

// Card carries the card brand (Consumer reads it from payments.methods.card.brand).
type Card struct {
	Brand string `json:"brand"`
}

// Cash carries cash payment details (Consumer reads it from payments.methods.cash.changeFor).
type Cash struct {
	ChangeFor float64 `json:"changeFor"`
}

// Transaction carries card authorization data.
type Transaction struct {
	AuthorizationCode string `json:"authorizationCode,omitempty"`
	AcquirerDocument  string `json:"acquirerDocument,omitempty"`
}

// Customer is the order customer.
type Customer struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	DocumentNumber        *string  `json:"documentNumber,omitempty"`
	Phone                 Phone    `json:"phone"`
	Email                 string   `json:"email,omitempty"`
	OrdersCountOnMerchant *float64 `json:"ordersCountOnMerchant,omitempty"`
}

// Phone is the contact phone.
type Phone struct {
	Number              string  `json:"number"`
	Extension           string  `json:"extension,omitempty"`
	Localizer           *string `json:"localizer,omitempty"`
	LocalizerExpiration *string `json:"localizerExpiration,omitempty"`
}

// Delivery groups the delivery data.
type Delivery struct {
	Mode             string  `json:"mode"`
	DeliveredBy      string  `json:"deliveredBy"`
	PickupCode       string  `json:"pickupCode"`
	DeliveryDateTime string  `json:"deliveryDateTime"`
	DeliveryAddress  Address `json:"deliveryAddress"`
}

// Address is the delivery address in the Consumer format.
type Address struct {
	Country          string      `json:"country"`
	State            string      `json:"state"`
	City             string      `json:"city"`
	PostalCode       string      `json:"postalCode"`
	StreetName       string      `json:"streetName"`
	StreetNumber     string      `json:"streetNumber"`
	Neighborhood     string      `json:"neighborhood"`
	Complement       string      `json:"complement,omitempty"`
	Reference        string      `json:"reference,omitempty"`
	FormattedAddress string      `json:"formattedAddress,omitempty"`
	Coordinates      Coordinates `json:"coordinates"`
}

// Coordinates are the address coordinates.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Takeout groups the pickup data.
type Takeout struct {
	Mode            string `json:"mode"`
	TakeoutDateTime string `json:"takeoutDateTime"`
}

// Indoor groups the dine-in data.
type Indoor struct {
	Mode           string `json:"mode"`
	IndoorDateTime string `json:"indoorDateTime"`
	Place          string `json:"place,omitempty"`
	Tab            string `json:"tab,omitempty"`
	Seat           string `json:"seat,omitempty"`
	WaiterCode     string `json:"waiterCode,omitempty"`
}
