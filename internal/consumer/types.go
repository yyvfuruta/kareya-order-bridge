// Package consumer define os schemas da API do parceiro esperados pelo
// Consumer (sistema de gestao do restaurante) e a traducao a partir do
// pedido da Brendi.
package consumer

// PollingResponse e o corpo de GET no endpoint de polling.
type PollingResponse struct {
	Items        []PollingEvent `json:"items"`
	StatusCode   int            `json:"statusCode"`
	ReasonPhrase *string        `json:"reasonPhrase"`
}

// PollingEvent e um evento de pedido novo.
type PollingEvent struct {
	ID        string `json:"id"`
	OrderID   string `json:"orderId"`
	CreatedAt string `json:"createdAt"`
	FullCode  string `json:"fullCode"`
	Code      string `json:"code"`
}

// OrderResponse e o corpo do endpoint de detalhes do pedido.
type OrderResponse struct {
	Item         Order   `json:"item"`
	StatusCode   int     `json:"statusCode"`
	ReasonPhrase *string `json:"reasonPhrase"`
}

// EventResponse e o corpo do endpoint de consulta de um evento.
type EventResponse struct {
	Item         PollingEvent `json:"item"`
	StatusCode   int          `json:"statusCode"`
	ReasonPhrase *string      `json:"reasonPhrase"`
}

// StoresResponse e o corpo do endpoint de listagem de lojas.
type StoresResponse struct {
	Items        []Store `json:"items"`
	StatusCode   int     `json:"statusCode"`
	ReasonPhrase *string `json:"reasonPhrase"`
}

// Store identifica uma loja do parceiro.
type Store struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// StatusRequest e o corpo enviado pelo Consumer ao alterar o status.
type StatusRequest struct {
	OrderID       string `json:"orderId"`
	Status        string `json:"status"`
	Justification string `json:"justification,omitempty"`
}

// Response e a resposta padrao de sucesso/erro das operacoes.
type Response struct {
	StatusCode   int     `json:"statusCode"`
	ReasonPhrase *string `json:"reasonPhrase"`
}

// Order e o pedido no formato esperado pelo Consumer.
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

// Merchant e a loja que recebeu o pedido.
type Merchant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Item e uma linha do pedido.
type Item struct {
	ID               string   `json:"id"`
	Index            int      `json:"index,omitempty"`
	Name             string   `json:"name"`
	ExternalCode     string   `json:"externalCode"`
	Unit             string   `json:"unit,omitempty"`
	Quantity         float64  `json:"quantity"`
	UnitPrice        float64  `json:"unitPrice"`
	TotalPrice       float64  `json:"totalPrice"`
	Observations     string   `json:"observations,omitempty"`
	ImageURL         string   `json:"imageUrl,omitempty"`
	Options          []Option `json:"options,omitempty"`
	ConsumerItemInfo any      `json:"consumerItemInfo,omitempty"`
}

// Option e um complemento do item.
type Option struct {
	Name         string  `json:"name"`
	ExternalCode string  `json:"externalCode,omitempty"`
	Quantity     float64 `json:"quantity"`
	UnitPrice    float64 `json:"unitPrice"`
	Price        float64 `json:"price"`
	Index        int     `json:"index,omitempty"`
}

// Benefit e um desconto aplicado ao pedido.
type Benefit struct {
	Target            string             `json:"target"`
	TargetID          string             `json:"targetId,omitempty"`
	Value             float64            `json:"value"`
	SponsorshipValues []SponsorshipValue `json:"sponsorshipValues,omitempty"`
}

// SponsorshipValue diz quem bancou o desconto.
type SponsorshipValue struct {
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	Description string  `json:"description,omitempty"`
}

// Total resume os valores do pedido.
type Total struct {
	SubTotal       float64 `json:"subTotal"`
	DeliveryFee    float64 `json:"deliveryFee"`
	Benefits       float64 `json:"benefits"`
	AdditionalFees float64 `json:"additionalFees"`
	OrderAmount    float64 `json:"orderAmount"`
}

// Payments descreve o pagamento do pedido.
type Payments struct {
	Prepaid float64   `json:"prepaid"`
	Pending float64   `json:"pending"`
	Methods []Payment `json:"methods"`
}

// Payment e uma forma de pagamento.
type Payment struct {
	Value       float64      `json:"value"`
	Currency    string       `json:"currency"`
	Type        string       `json:"type"`
	Method      string       `json:"method"`
	Brand       string       `json:"brand,omitempty"`
	MethodInfo  string       `json:"methodInfo,omitempty"`
	Card        *Card        `json:"card,omitempty"`
	Transaction *Transaction `json:"transaction,omitempty"`
	ChangeFor   float64      `json:"changeFor,omitempty"`
}

// Card traz a bandeira do cartao (o Consumer le em payments.methods.card.brand).
type Card struct {
	Brand string `json:"brand"`
}

// Transaction traz dados de autorizacao do cartao.
type Transaction struct {
	AuthorizationCode string `json:"authorizationCode,omitempty"`
	AcquirerDocument  string `json:"acquirerDocument,omitempty"`
}

// Customer e o cliente do pedido.
type Customer struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	DocumentNumber        string  `json:"documentNumber,omitempty"`
	Phone                 Phone   `json:"phone"`
	Email                 string  `json:"email,omitempty"`
	OrdersCountOnMerchant float64 `json:"ordersCountOnMerchant"`
}

// Phone e o telefone de contato.
type Phone struct {
	Number              string `json:"number"`
	Extension           string `json:"extension,omitempty"`
	Localizer           string `json:"localizer"`
	LocalizerExpiration string `json:"localizerExpiration"`
}

// Delivery agrupa os dados de entrega.
type Delivery struct {
	Mode             string  `json:"mode"`
	DeliveredBy      string  `json:"deliveredBy"`
	PickupCode       string  `json:"pickupCode"`
	DeliveryDateTime string  `json:"deliveryDateTime"`
	DeliveryAddress  Address `json:"deliveryAddress"`
}

// Address e o endereco de entrega no formato do Consumer.
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

// Coordinates sao as coordenadas do endereco.
type Coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Takeout agrupa os dados de retirada.
type Takeout struct {
	Mode            string `json:"mode"`
	TakeoutDateTime string `json:"takeoutDateTime"`
}

// Indoor agrupa os dados de consumo no salao.
type Indoor struct {
	Mode           string `json:"mode"`
	IndoorDateTime string `json:"indoorDateTime"`
	Place          string `json:"place,omitempty"`
	Tab            string `json:"tab,omitempty"`
	Seat           string `json:"seat,omitempty"`
	WaiterCode     string `json:"waiterCode,omitempty"`
}
