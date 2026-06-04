package models

import "time"

// TimeOfUseRate represents a time-based electricity rate
type TimeOfUseRate struct {
	Name        string  // e.g., "Designated Free Period", "Paid Energy Period"
	RateCents   float64 // Rate in cents per kWh
	DayOfWeek   string  // "Monday-Friday", "Saturday", "Sunday", or "All"
	StartHour   int     // 0-23
	EndHour     int     // 0-23
	Description string  // Full description for reporting
}

// ChargeComponent represents a billing charge
type ChargeComponent struct {
	Name          string  // e.g., "Energy Charge", "Base Fee", "Delivery Charge"
	MonthlyFixed  float64 // Fixed monthly charge in dollars
	PerKwhCents   float64 // Per kWh charge in cents
	PerKwhDollars float64 // Per kWh charge in dollars
	Description   string
}

// Plan represents an electricity rate plan
type Plan struct {
	CompanyName           string
	PlanName              string
	ProductType           string // "Time Of Use", "Fixed", "Variable"
	ContractTermMonths    int
	TerminationFeeDollars float64

	// Pricing components
	EnergyCharge   ChargeComponent
	BaseCharge     ChargeComponent
	DeliveryCharge ChargeComponent

	// Time-of-use rates (if applicable)
	TimeOfUseRates []TimeOfUseRate

	// Other info
	RenewablePercent float64
	AvgPricePerKwh   float64 // Average price in cents/kWh
	PUCTCertificate  string
	IssueDate        time.Time
	ServiceArea      string

	// Observations
	Observations []string

	// Raw text for reference
	RawText string
}

// ConsumptionRecord represents a point in time consumption data
type ConsumptionRecord struct {
	Timestamp time.Time
	TotalKwh  float64
	Details   map[string]float64 // Individual circuit consumption
}

// MonthBill represents a calculated bill for a month
type MonthBill struct {
	Month               time.Time
	PlanName            string
	CompanyName         string
	TotalConsumptionKwh float64

	// Cost breakdown
	EnergyChargeDollars   float64
	BaseChargeDollars     float64
	DeliveryChargeDollars float64
	TotalBillDollars      float64

	// Calculation details
	CalculationSteps []CalculationStep
	DailyBreakdowns  []DailyBreakdown
}

// CalculationStep represents a step in calculating the bill
type CalculationStep struct {
	Description string
	KwhAmount   float64
	RateCents   float64
	CostDollars float64
}

// DailyBreakdown shows consumption and cost per day
type DailyBreakdown struct {
	Date           time.Time
	DayOfWeek      string
	ConsumptionKwh float64
	CostDollars    float64
	RatesApplied   []string
}

// ComparisonResult aggregates all plans and bills
type ComparisonResult struct {
	Plans        []Plan
	Consumptions map[string][]ConsumptionRecord  // filename -> records
	Bills        map[string]map[string]MonthBill // planName -> (month -> bill)
	Observations []string
}
