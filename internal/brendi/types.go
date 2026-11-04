package brendi

import (
	"encoding/json"
	"time"
)

// DateTime accepts the date formats used by Brendi/Open Delivery without
// breaking the parse of the whole payload. The original value is kept in Raw.
type DateTime struct {
	time.Time
	Raw string
}

var dateLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

func (d *DateTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	d.Raw = s
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			d.Time = t
			return nil
		}
	}
	// Unknown format: keep the text and do not fail the whole order.
	return nil
}

func (d DateTime) MarshalJSON() ([]byte, error) {
	if d.Raw != "" {
		return json.Marshal(d.Raw)
	}
	if d.Time.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(d.Time.Format(time.RFC3339Nano))
}

func (d DateTime) String() string {
	if d.Raw != "" {
		return d.Raw
	}
	if d.Time.IsZero() {
		return ""
	}
	return d.Time.Format(time.RFC3339)
}

// Valid reports whether the date was recognized as a valid instant.
func (d DateTime) Valid() bool { return !d.Time.IsZero() }

// Price is a monetary value with currency, as returned by the API.
type Price struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

// Enums from the Open Delivery standard followed by the Brendi API.

type OrderType string

const (
	OrderTypeDelivery OrderType = "DELIVERY"
	OrderTypeTakeout  OrderType = "TAKEOUT"
	OrderTypeIndoor   OrderType = "INDOOR"
)

type OrderTiming string

const (
	OrderTimingInstant   OrderTiming = "INSTANT"
	OrderTimingScheduled OrderTiming = "SCHEDULED"
	OrderTimingOnDemand  OrderTiming = "ONDEMAND"
)

type OrderCategory string

const (
	CategoryFood      OrderCategory = "FOOD"
	CategoryGrocery   OrderCategory = "GROCERY"
	CategoryDrugstore OrderCategory = "DRUGSTORE"
	CategoryPetstore  OrderCategory = "PETSTORE"
)

type OrderPriority string

const (
	Priority1 OrderPriority = "PRIORITY1"
	Priority2 OrderPriority = "PRIORITY2"
	Priority3 OrderPriority = "PRIORITY3"
	Priority4 OrderPriority = "PRIORITY4"
	Priority5 OrderPriority = "PRIORITY5"
)

type EventType string

const (
	EventCreated                   EventType = "CREATED"
	EventConfirmed                 EventType = "CONFIRMED"
	EventPreparationRequested      EventType = "PREPARATION_REQUESTED"
	EventPreparing                 EventType = "PREPARING"
	EventDispatched                EventType = "DISPATCHED"
	EventReadyForPickup            EventType = "READY_FOR_PICKUP"
	EventPickupAreaAssigned        EventType = "PICKUP_AREA_ASSIGNED"
	EventPickedUp                  EventType = "PICKED_UP"
	EventDelivered                 EventType = "DELIVERED"
	EventConcluded                 EventType = "CONCLUDED"
	EventCancellationRequested     EventType = "CANCELLATION_REQUESTED"
	EventCancellationRequestDenied EventType = "CANCELLATION_REQUEST_DENIED"
	EventCancelled                 EventType = "CANCELLED"
	EventOrderCancellationRequest  EventType = "ORDER_CANCELLATION_REQUEST"
	EventCancelledDenied           EventType = "CANCELLED_DENIED"
)

type ItemUnit string

const (
	UnitUN  ItemUnit = "UN"
	UnitKG  ItemUnit = "KG"
	UnitL   ItemUnit = "L"
	UnitOZ  ItemUnit = "OZ"
	UnitLB  ItemUnit = "LB"
	UnitGAL ItemUnit = "GAL"
)

type FeeType string

const (
	FeeDelivery FeeType = "DELIVERY_FEE"
	FeeService  FeeType = "SERVICE_FEE"
	FeeTip      FeeType = "TIP"
	FeeMinOrder FeeType = "MIN_ORDER_FEE"
)

type FeeReceiver string

const (
	ReceivedByMarketplace FeeReceiver = "MARKETPLACE"
	ReceivedByMerchant    FeeReceiver = "MERCHANT"
	ReceivedByLogistics   FeeReceiver = "LOGISTIC_SERVICES"
)

type DiscountTarget string

const (
	DiscountTargetCart DiscountTarget = "CART"
	DiscountTargetFee  DiscountTarget = "DELIVERY_FEE"
	DiscountTargetItem DiscountTarget = "ITEM"
)

type SponsorshipName string

const (
	SponsorshipMarketplace SponsorshipName = "MARKETPLACE"
	SponsorshipMerchant    SponsorshipName = "MERCHANT"
	SponsorshipChain       SponsorshipName = "CHAIN"
)

type PaymentType string

const (
	PaymentTypePrepaid PaymentType = "PREPAID"
	PaymentTypePending PaymentType = "PENDING"
)

type PaymentMethod string

const (
	PaymentCredit        PaymentMethod = "CREDIT"
	PaymentDebit         PaymentMethod = "DEBIT"
	PaymentMealVoucher   PaymentMethod = "MEAL_VOUCHER"
	PaymentFoodVoucher   PaymentMethod = "FOOD_VOUCHER"
	PaymentDigitalWallet PaymentMethod = "DIGITAL_WALLET"
	PaymentPix           PaymentMethod = "PIX"
	PaymentCash          PaymentMethod = "CASH"
	PaymentCreditDebit   PaymentMethod = "CREDIT_DEBIT"
	PaymentCoupon        PaymentMethod = "COUPON"
	PaymentRedeem        PaymentMethod = "REDEEM"
	PaymentPrepaidRedeem PaymentMethod = "PREPAID_REDEEM"
	PaymentOther         PaymentMethod = "OTHER"
)

type PaymentBrand string

const (
	BrandVisa       PaymentBrand = "VISA"
	BrandMastercard PaymentBrand = "MASTERCARD"
	BrandDiners     PaymentBrand = "DINERS"
	BrandAmex       PaymentBrand = "AMEX"
	BrandHipercard  PaymentBrand = "HIPERCARD"
	BrandElo        PaymentBrand = "ELO"
	BrandAura       PaymentBrand = "AURA"
	BrandDiscover   PaymentBrand = "DISCOVER"
	BrandVR         PaymentBrand = "VR_BENEFICIOS"
	BrandSodexo     PaymentBrand = "SODEXO"
	BrandTicket     PaymentBrand = "TICKET"
	BrandGoodCard   PaymentBrand = "GOOD_CARD"
	BrandBanescard  PaymentBrand = "BANESCARD"
	BrandSorocard   PaymentBrand = "SOROCARD"
	BrandPolicard   PaymentBrand = "POLICARD"
	BrandValecard   PaymentBrand = "VALECARD"
	BrandAgicard    PaymentBrand = "AGICARD"
	BrandJCB        PaymentBrand = "JCB"
	BrandCredsystem PaymentBrand = "CREDSYSTEM"
	BrandCabal      PaymentBrand = "CABAL"
	BrandGreenCard  PaymentBrand = "GREEN_CARD"
	BrandVerocheque PaymentBrand = "VEROCHEQUE"
	BrandAvista     PaymentBrand = "AVISTA"
	BrandOther      PaymentBrand = "OTHER"
)

type DeliveredBy string

const (
	DeliveredByMerchant    DeliveredBy = "MERCHANT"
	DeliveredByMarketplace DeliveredBy = "MARKETPLACE"
)

type TakeoutMode string

const (
	TakeoutModeDefault    TakeoutMode = "DEFAULT"
	TakeoutModePickupArea TakeoutMode = "PICKUP_AREA"
)

type IndoorMode string

const (
	IndoorModeDefault  IndoorMode = "DEFAULT"
	IndoorModePlace    IndoorMode = "PLACE"
	IndoorModeTab      IndoorMode = "TAB"
	IndoorModeTerminal IndoorMode = "TERMINAL"
)
