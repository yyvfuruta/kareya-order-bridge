package brendi

// Order e o pedido detalhado retornado por GET /v1/orders/{orderId}.
// O formato segue o padrao Open Delivery (v1), usado pela API Publica da Brendi.
// Campos com `omitempty` sao opcionais no schema; os demais sao obrigatorios.
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

// TotalAmount devolve o valor total do pedido.
func (o Order) TotalAmount() float64 { return o.Total.OrderAmount.Value }

// IsDelivery informa se o pedido deve ser entregue no endereco do cliente.
func (o Order) IsDelivery() bool { return o.Type == OrderTypeDelivery }

// Merchant identifica a loja que recebeu o pedido.
type Merchant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// OrderItem e uma linha do pedido (produto + complementos).
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

// ItemOption e um complemento/additional escolhido para um item.
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

// ItemIndoor aponta o ponto de producao de um item consumido no salao.
type ItemIndoor struct {
	ProductionPoint string `json:"productionPoint,omitempty"`
}

// OtherFee e uma taxa extra (entrega, servico, gorjeta, pedido minimo).
type OtherFee struct {
	Name             string      `json:"name"`
	Type             FeeType     `json:"type"`
	ReceivedBy       FeeReceiver `json:"receivedBy"`
	ReceiverDocument string      `json:"receiverDocument,omitempty"`
	Price            Price       `json:"price"`
	Observation      string      `json:"observation,omitempty"`
}

// Discount e um desconto aplicado ao carrinho, a um item ou a taxa de entrega.
type Discount struct {
	Amount            Price                 `json:"amount"`
	Target            DiscountTarget        `json:"target"`
	TargetID          string                `json:"targetId,omitempty"`
	SponsorshipValues []DiscountSponsorship `json:"sponsorshipValues"`
}

// DiscountSponsorship diz quem bancou o desconto.
type DiscountSponsorship struct {
	Name         SponsorshipName `json:"name"`
	Amount       Price           `json:"amount"`
	DiscountCode string          `json:"discountCode,omitempty"`
}

// OrderTotal resume os valores do pedido.
type OrderTotal struct {
	ItemsPrice  Price `json:"itemsPrice"`
	OtherFees   Price `json:"otherFees"`
	Discount    Price `json:"discount"`
	OrderAmount Price `json:"orderAmount"`
}

// Payments descreve o que ja foi pago e o que falta pagar.
type Payments struct {
	Prepaid float64   `json:"prepaid"`
	Pending float64   `json:"pending"`
	Methods []Payment `json:"methods"`
}

// Payment e uma forma de pagamento do pedido.
type Payment struct {
	Value       float64             `json:"value"`
	Currency    string              `json:"currency"`
	Type        PaymentType         `json:"type"`
	Method      PaymentMethod       `json:"method"`
	Brand       PaymentBrand        `json:"brand,omitempty"`
	MethodInfo  string              `json:"methodInfo,omitempty"`
	Transaction *PaymentTransaction `json:"transaction,omitempty"`
	ChangeFor   float64             `json:"changeFor,omitempty"`
}

// PaymentTransaction traz dados da transacao quando houver.
type PaymentTransaction struct {
	AuthorizationCode string `json:"authorizationCode,omitempty"`
	AcquirerDocument  string `json:"acquirerDocument,omitempty"`
}

// TaxInvoice indica se a nota fiscal foi emitida.
type TaxInvoice struct {
	Issued        bool   `json:"issued"`
	TaxInvoiceURL string `json:"taxInvoiceURL,omitempty"`
}

// Customer e o cliente que fez o pedido.
type Customer struct {
	ID                    string        `json:"id"`
	Name                  string        `json:"name"`
	DocumentNumber        string        `json:"documentNumber,omitempty"`
	Phone                 CustomerPhone `json:"phone"`
	Email                 string        `json:"email,omitempty"`
	OrdersCountOnMerchant float64       `json:"ordersCountOnMerchant"`
}

// CustomerPhone e o telefone de contato do cliente.
type CustomerPhone struct {
	Number    string `json:"number"`
	Extension string `json:"extension,omitempty"`
}

// Schedule indica a janela de um pedido agendado.
type Schedule struct {
	ScheduledDateTimeStart DateTime `json:"scheduledDateTimeStart"`
	ScheduledDateTimeEnd   DateTime `json:"scheduledDateTimeEnd"`
}

// Delivery agrupa os dados de entrega do pedido.
type Delivery struct {
	DeliveredBy               DeliveredBy     `json:"deliveredBy"`
	DeliveryAddress           DeliveryAddress `json:"deliveryAddress"`
	DeliveryFee               *Price          `json:"deliveryFee,omitempty"`
	EstimatedDeliveryDateTime DateTime        `json:"estimatedDeliveryDateTime"`
	DeliveryDateTime          DateTime        `json:"deliveryDateTime,omitempty"`
	PickupCode                string          `json:"pickupCode,omitempty"`
}

// DeliveryAddress e o endereco de entrega.
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

// Coordinates sao as coordenadas geograficas do endereco.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Takeout agrupa os dados de retirada no balcao.
type Takeout struct {
	Mode            TakeoutMode     `json:"mode"`
	TakeoutDateTime DateTime        `json:"takeoutDateTime"`
	BillingAddress  *BillingAddress `json:"billingAddress,omitempty"`
}

// BillingAddress e o endereco de cobranca (opcional no takeout).
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

// Indoor agrupa os dados de consumo no salao.
type Indoor struct {
	Mode           IndoorMode `json:"mode"`
	IndoorDateTime DateTime   `json:"indoorDateTime"`
	Place          string     `json:"place,omitempty"`
	Tab            string     `json:"tab,omitempty"`
	Seat           string     `json:"seat,omitempty"`
	WaiterCode     string     `json:"waiterCode,omitempty"`
}
