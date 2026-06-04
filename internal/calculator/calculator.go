package calculator

import (
	"fmt"
	"sort"
	"time"

	"electriccompare/internal/models"
)

// CalculateBills computes bills for a plan based on consumption data
func CalculateBills(plan *models.Plan, records []models.ConsumptionRecord) map[string]models.MonthBill {
	if len(records) == 0 {
		return make(map[string]models.MonthBill)
	}

	// Group records by month
	monthlyConsumption := groupByMonth(records)

	bills := make(map[string]models.MonthBill)

	for monthKey, dayRecords := range monthlyConsumption {
		bill := models.MonthBill{
			PlanName:    plan.PlanName,
			CompanyName: plan.CompanyName,
			DailyBreakdowns: []models.DailyBreakdown{},
			CalculationSteps: []models.CalculationStep{},
		}

		// Parse month key
		monthTime, _ := time.Parse("2006-01", monthKey)
		bill.Month = monthTime

		// Group by day for breakdown
		dailyGroups := groupByDay(dayRecords)

		totalConsumption := 0.0
		totalEnergyCost := 0.0
		totalDeliveryCost := 0.0

		for _, dayKey := range getSortedDays(dailyGroups) {
			dayRecords := dailyGroups[dayKey]
			dayDate, _ := time.Parse("2006-01-02", dayKey)
			
			dayConsumption := 0.0
			dayCost := 0.0
			ratesApplied := make(map[string]bool)

			// Group by hour for time-of-use pricing
			if plan.ProductType == "Time Of Use" && len(plan.TimeOfUseRates) > 0 {
				hourlyGroups := groupByHour(dayRecords)
				for _, hourKey := range getSortedHours(hourlyGroups) {
					hourRecords := hourlyGroups[hourKey]
					hourTime, _ := time.Parse("2006-01-02 15", hourKey)

					hourConsumption := 0.0
					for _, record := range hourRecords {
						hourConsumption += record.TotalKwh
					}

					// Find applicable rate
					rate := getTimeOfUseRate(plan, hourTime)
					rateCost := (rate.RateCents / 100) * hourConsumption
					dayCost += rateCost
					totalEnergyCost += rateCost

					dayConsumption += hourConsumption
					ratesApplied[rate.Name] = true

					bill.CalculationSteps = append(bill.CalculationSteps, models.CalculationStep{
						Description: fmt.Sprintf("%s (%s) - %v", rate.Name, hourTime.Format("15:00"), dayDate.Format("2006-01-02")),
						KwhAmount:   hourConsumption,
						RateCents:   rate.RateCents,
						CostDollars: rateCost,
					})
				}
			} else {
				// Fixed rate pricing
				for _, record := range dayRecords {
					dayConsumption += record.TotalKwh
				}

				energyCost := (plan.EnergyCharge.PerKwhDollars) * dayConsumption
				dayCost += energyCost
				totalEnergyCost += energyCost
				ratesApplied["Energy Charge"] = true

				bill.CalculationSteps = append(bill.CalculationSteps, models.CalculationStep{
					Description: fmt.Sprintf("Energy Charge - %s", dayDate.Format("2006-01-02")),
					KwhAmount:   dayConsumption,
						RateCents:   plan.EnergyCharge.PerKwhCents,
					CostDollars: energyCost,
				})
			}

			// Add delivery charge for this day (will be rolled up)
			dailyDeliveryCost := plan.DeliveryCharge.MonthlyFixed/30 + (plan.DeliveryCharge.PerKwhDollars * dayConsumption)
			dayCost += dailyDeliveryCost

			totalConsumption += dayConsumption
			totalDeliveryCost += dailyDeliveryCost

			ratesList := []string{}
			for rate := range ratesApplied {
				ratesList = append(ratesList, rate)
			}
			sort.Strings(ratesList)

			bill.DailyBreakdowns = append(bill.DailyBreakdowns, models.DailyBreakdown{
				Date:              dayDate,
				DayOfWeek:         dayDate.Weekday().String(),
				ConsumptionKwh:    dayConsumption,
				CostDollars:       dayCost,
				RatesApplied:      ratesList,
			})
		}

		bill.TotalConsumptionKwh = totalConsumption
		bill.EnergyChargeDollars = totalEnergyCost
		bill.DeliveryChargeDollars = totalDeliveryCost
		bill.BaseChargeDollars = plan.BaseCharge.MonthlyFixed

		bill.TotalBillDollars = bill.BaseChargeDollars + bill.EnergyChargeDollars + bill.DeliveryChargeDollars

		bills[monthKey] = bill
	}

	return bills
}

func getTimeOfUseRate(plan *models.Plan, t time.Time) models.TimeOfUseRate {
	// Default to first rate if not found
	defaultRate := models.TimeOfUseRate{
		Name:        "Default",
		RateCents:   plan.EnergyCharge.PerKwhCents,
		Description: "Default rate",
	}

	if len(plan.TimeOfUseRates) == 0 {
		return defaultRate
	}

	hour := t.Hour()
	dayOfWeek := t.Weekday().String()

	// Check each rate
	for _, rate := range plan.TimeOfUseRates {
		// Check day of week
		dayMatch := false
		if rate.DayOfWeek == "All" {
			dayMatch = true
		} else if rate.DayOfWeek == "Monday-Friday" && dayOfWeek != "Saturday" && dayOfWeek != "Sunday" {
			dayMatch = true
		} else if rate.DayOfWeek == "Saturday" && dayOfWeek == "Saturday" {
			dayMatch = true
		} else if rate.DayOfWeek == "Sunday" && dayOfWeek == "Sunday" {
			dayMatch = true
		}

		if dayMatch {
			// Check hour range
			if rate.StartHour <= rate.EndHour {
				if hour >= rate.StartHour && hour < rate.EndHour {
					return rate
				}
			} else {
				// Wraps around midnight (e.g., 10pm-2am)
				if hour >= rate.StartHour || hour < rate.EndHour {
					return rate
				}
			}
		}
	}

	// If no specific rate matched, return paid rate (non-free)
	for _, rate := range plan.TimeOfUseRates {
		if rate.Name != "Designated Free Period" && rate.RateCents > 0 {
			return rate
		}
	}

	return defaultRate
}

func groupByMonth(records []models.ConsumptionRecord) map[string][]models.ConsumptionRecord {
	groups := make(map[string][]models.ConsumptionRecord)
	for _, record := range records {
		key := record.Timestamp.Format("2006-01")
		groups[key] = append(groups[key], record)
	}
	return groups
}

func groupByDay(records []models.ConsumptionRecord) map[string][]models.ConsumptionRecord {
	groups := make(map[string][]models.ConsumptionRecord)
	for _, record := range records {
		key := record.Timestamp.Format("2006-01-02")
		groups[key] = append(groups[key], record)
	}
	return groups
}

func groupByHour(records []models.ConsumptionRecord) map[string][]models.ConsumptionRecord {
	groups := make(map[string][]models.ConsumptionRecord)
	for _, record := range records {
		key := record.Timestamp.Format("2006-01-02 15")
		groups[key] = append(groups[key], record)
	}
	return groups
}

func getSortedDays(groups map[string][]models.ConsumptionRecord) []string {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func getSortedHours(groups map[string][]models.ConsumptionRecord) []string {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
