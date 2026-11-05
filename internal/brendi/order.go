package brendi

// Order is the detailed order returned by GET /v1/orders/{orderId}.
// The format follows the Open Delivery (v1) standard used by Brendi's Public API.
// Fields with `omitempty` are optional in the schema; the others are mandatory.
type Order struct {
	ID                       string        `json:"id"`
	Type                     OrderType     `json:"type"`
	DisplayID                string        `json:"displayId"`
	SourceOrderID            string        `json:"sourceOrderId,omitempty"`
	SourceAppID              string        `json:"sourceAppId,omitempty"`
	SalesChannel             string        `json:"salesChannel,omitempty"`
	VirtualBrand             string        `json:"virtualBrand,omitempty"`
	Category                 OrderCategory `json:"category,omitempty"`
	CreatedAt                DateTime      `json:"createdAt"`
	LastEvent                EventType     `json:"lastEvent,omitempty"`
	OrderTiming              OrderTiming   `json:"orderTiming"`
	PreparationStartDateTime DateTime      `json:"preparationStartDateTime"`
	Merchant                 Merchant      `json:"merchant"`
	Items                    []OrderItem   `json:"items"`
	OtherFees                []OtherFee    `json:"otherFees,omitempty"`
	Discounts                []Discount    `json:"discounts,omitempty"`
	Total                    OrderTotal    `json:"total"`
	Payments                 Payments      `json:"payments"`
	TaxInvoice               *TaxInvoice   `json:"taxInvoice,omitempty"`
	Customer                 *Customer     `json:"customer,omitempty"`
	Schedule                 *Schedule     `json:"schedule,omitempty"`
	OrderPriority            OrderPriority `json:"orderPriority,omitempty"`
	Delivery                 *Delivery     `json:"delivery,omitempty"`
	Takeout                  *Takeout      `json:"takeout,omitempty"`
	Indoor                   *Indoor       `json:"indoor,omitempty"`
	SendPreparing            bool          `json:"sendPreparing,omitempty"`
	SendDelivered            bool          `json:"sendDelivered,omitempty"`
	SendPickedUp             bool          `json:"sendPickedUp,omitempty"`
	SendTracking             bool          `json:"sendTracking,omitempty"`
	ExtraInfo                string        `json:"extraInfo,omitempty"`
}

// TotalAmount returns the order total.
func (o Order) TotalAmount() float64 { return o.Total.OrderAmount.Value }

// IsDelivery reports whether the order must be delivered to the customer address.
func (o Order) IsDelivery() bool { return o.Type == OrderTypeDelivery }

// Merchant identifies the store that received the order.
type Merchant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// OrderItem is an order line (product + options).
type OrderItem struct {
	ID                  string       `json:"id"`
	Index               int          `json:"index,omitempty"`
	Name                string       `json:"name"`
	ExternalCode        string       `json:"externalCode"`
	Unit                ItemUnit     `json:"unit"`
	EAN                 string       `json:"ean,omitempty"`
	Quantity            float64      `json:"quantity"`
	SpecialInstructions string       `json:"specialInstructions,omitempty"`
	UnitPrice           Price        `json:"unitPrice"`
	OriginalPrice       *Price       `json:"originalPrice,omitempty"`
	ScalePriceApplied   bool         `json:"scalePriceApplied,omitempty"`
	OptionsPrice        *Price       `json:"optionsPrice,omitempty"`
	SubtotalPrice       *Price       `json:"subtotalPrice,omitempty"`
	TotalPrice          Price        `json:"totalPrice"`
	Indoor              *ItemIndoor  `json:"indoor,omitempty"`
	Options             []ItemOption `json:"options,omitempty"`
}

// ItemOption is an option/add-on chosen for an item.
type ItemOption struct {
	ID                  string   `json:"id"`
	Index               int      `json:"index,omitempty"`
	Name                string   `json:"name"`
	ExternalCode        string   `json:"externalCode"`
	Unit                ItemUnit `json:"unit"`
	EAN                 string   `json:"ean,omitempty"`
	Quantity            float64  `json:"quantity"`
	UnitPrice           Price    `json:"unitPrice"`
	OriginalPrice       *Price   `json:"originalPrice,omitempty"`
	SubtotalPrice       *Price   `json:"subtotalPrice,omitempty"`
	TotalPrice          Price    `json:"totalPrice"`
	SpecialInstructions string   `json:"specialInstructions,omitempty"`
}

// ItemIndoor points to the production point of an item consumed in the dining room.
type ItemIndoor struct {
	ProductionPoint string `json:"productionPoint,omitempty"`
}

// OtherFee is an extra fee (delivery, service, tip, minimum order).
type OtherFee struct {
	Name             string      `json:"name"`
	Type             FeeType     `json:"type"`
	ReceivedBy       FeeReceiver `json:"receivedBy"`
	ReceiverDocument string      `json:"receiverDocument,omitempty"`
	Price            Price       `json:"price"`
	Observation      string      `json:"observation,omitempty"`
}

// Discount is a discount applied to the cart, an item, or the delivery fee.
type Discount struct {
	Amount            Price                 `json:"amount"`
	Target            DiscountTarget        `json:"target"`
	TargetID          string                `json:"targetId,omitempty"`
	SponsorshipValues []DiscountSponsorship `json:"sponsorshipValues"`
}

// DiscountSponsorship says who funded the discount.
type DiscountSponsorship struct {
	Name         SponsorshipName `json:"name"`
	Amount       Price           `json:"amount"`
	DiscountCode string          `json:"discountCode,omitempty"`
}

// OrderTotal summarizes the order amounts.
type OrderTotal struct {
	ItemsPrice  Price `json:"itemsPrice"`
	OtherFees   Price `json:"otherFees"`
	Discount    Price `json:"discount"`
	OrderAmount Price `json:"orderAmount"`
}

// Payments describes what was already paid and what is still pending.
type Payments struct {
	Prepaid float64   `json:"prepaid"`
	Pending float64   `json:"pending"`
	Methods []Payment `json:"methods"`
}

// Payment is an order payment method.
type Payment struct {
	Value       float64             `json:"value"`
	Currency    string              `json:"currency"`
	Type        PaymentType         `json:"type"`
	Method      PaymentMethod       `json:"method"`
	Prepaid     bool                `json:"prepaid,omitempty"`
	Brand       PaymentBrand        `json:"brand,omitempty"`
	MethodInfo  string              `json:"methodInfo,omitempty"`
	Transaction *PaymentTransaction `json:"transaction,omitempty"`
	ChangeFor   float64             `json:"changeFor,omitempty"`
}

// PaymentTransaction carries transaction data when available.
type PaymentTransaction struct {
	AuthorizationCode string `json:"authorizationCode,omitempty"`
	AcquirerDocument  string `json:"acquirerDocument,omitempty"`
}

// TaxInvoice indicates whether the invoice was issued.
type TaxInvoice struct {
	Issued        bool   `json:"issued"`
	TaxInvoiceURL string `json:"taxInvoiceURL,omitempty"`
}

// Customer is the customer who placed the order.
type Customer struct {
	ID                    string        `json:"id"`
	Name                  string        `json:"name"`
	DocumentNumber        string        `json:"documentNumber,omitempty"`
	Phone                 CustomerPhone `json:"phone"`
	Email                 string        `json:"email,omitempty"`
	OrdersCountOnMerchant float64       `json:"ordersCountOnMerchant"`
}

// CustomerPhone is the customer contact phone.
type CustomerPhone struct {
	Number              string   `json:"number"`
	Extension           string   `json:"extension,omitempty"`
	Localizer           string   `json:"localizer,omitempty"`
	LocalizerExpiration DateTime `json:"localizerExpiration,omitempty"`
}

// Schedule indicates the window of a scheduled order.
type Schedule struct {
	ScheduledDateTimeStart DateTime `json:"scheduledDateTimeStart"`
	ScheduledDateTimeEnd   DateTime `json:"scheduledDateTimeEnd"`
}

// Delivery groups the order delivery data.
type Delivery struct {
	DeliveredBy               DeliveredBy     `json:"deliveredBy"`
	DeliveryAddress           DeliveryAddress `json:"deliveryAddress"`
	DeliveryFee               *Price          `json:"deliveryFee,omitempty"`
	EstimatedDeliveryDateTime DateTime        `json:"estimatedDeliveryDateTime"`
	DeliveryDateTime          DateTime        `json:"deliveryDateTime,omitempty"`
	PickupCode                string          `json:"pickupCode,omitempty"`
}

// DeliveryAddress is the delivery address.
type DeliveryAddress struct {
	Country          string      `json:"country"`
	State            string      `json:"state"`
	City             string      `json:"city"`
	District         string      `json:"district"`
	Street           string      `json:"street"`
	Number           string      `json:"number"`
	Complement       string      `json:"complement,omitempty"`
	Reference        string      `json:"reference,omitempty"`
	FormattedAddress string      `json:"formattedAddress"`
	PostalCode       string      `json:"postalCode"`
	Coordinates      Coordinates `json:"coordinates"`
}

// Coordinates are the address geographic coordinates.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Takeout groups the counter pickup data.
type Takeout struct {
	Mode            TakeoutMode     `json:"mode"`
	TakeoutDateTime DateTime        `json:"takeoutDateTime"`
	BillingAddress  *BillingAddress `json:"billingAddress,omitempty"`
}

// BillingAddress is the billing address (optional for takeout).
type BillingAddress struct {
	Country    string `json:"country"`
	State      string `json:"state"`
	City       string `json:"city"`
	District   string `json:"district"`
	Street     string `json:"street"`
	Number     string `json:"number"`
	PostalCode string `json:"postalCode"`
	Complement string `json:"complement,omitempty"`
}

// Indoor groups the dine-in data.
type Indoor struct {
	Mode           IndoorMode `json:"mode"`
	IndoorDateTime DateTime   `json:"indoorDateTime"`
	Place          string     `json:"place,omitempty"`
	Tab            string     `json:"tab,omitempty"`
	Seat           string     `json:"seat,omitempty"`
	WaiterCode     string     `json:"waiterCode,omitempty"`
}
