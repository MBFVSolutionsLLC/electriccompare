package output

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"electriccompare/internal/models"

	"github.com/xuri/excelize/v2"
)

// GenerateOutput creates Excel files and summary reports
func GenerateOutput(result *models.ComparisonResult, outputDir string) error {
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate summary spreadsheet
	if err := generateSummarySpreadsheet(result, filepath.Join(outputDir, "Plan_Comparison_Summary.xlsx")); err != nil {
		return fmt.Errorf("failed to generate summary: %w", err)
	}

	// Generate per-plan reports
	if err := generatePerPlanReports(result, outputDir); err != nil {
		return fmt.Errorf("failed to generate per-plan reports: %w", err)
	}

	return nil
}

func generateSummarySpreadsheet(result *models.ComparisonResult, filePath string) error {
	f := excelize.NewFile()
	defer f.Close()

	// Create summary sheet
	f.SetSheetName("Sheet1", "Plan Summary")

	// Headers
	headers := []string{
		"Company Name",
		"Plan Name",
		"Product Type",
		"Contract Term (Months)",
		"Energy Charge (¢/kWh)",
		"Base Fee ($/month)",
		"Delivery Fixed ($/month)",
		"Delivery Variable (¢/kWh)",
		"Termination Fee ($)",
		"Renewable %",
		"Time-of-Use",
		"Observations",
	}

	for col, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+rune(col))
		f.SetCellValue("Plan Summary", cell, header)
	}

	// Add plans
	for row, plan := range result.Plans {
		rowNum := row + 2
		cells := []interface{}{
			plan.CompanyName,
			plan.PlanName,
			plan.ProductType,
			plan.ContractTermMonths,
			plan.EnergyCharge.PerKwhCents,
			plan.BaseCharge.MonthlyFixed,
			plan.DeliveryCharge.MonthlyFixed,
			plan.DeliveryCharge.PerKwhCents,
			plan.TerminationFeeDollars,
			plan.RenewablePercent,
			fmt.Sprintf("%d rates", len(plan.TimeOfUseRates)),
			strings.Join(plan.Observations, "; "),
		}

		for col, val := range cells {
			cell := fmt.Sprintf("%c%d", 'A'+rune(col), rowNum)
			f.SetCellValue("Plan Summary", cell, val)
		}
	}

	// Set column widths
	colWidths := []float64{15, 25, 15, 15, 18, 15, 18, 18, 15, 12, 20, 40}
	for col, width := range colWidths {
		f.SetColWidth("Plan Summary", fmt.Sprintf("%c", 'A'+rune(col)), fmt.Sprintf("%c", 'A'+rune(col)), width)
	}

	// Create pricing details sheet
	createPricingDetailsSheet(f, result)

	if err := f.SaveAs(filePath); err != nil {
		return fmt.Errorf("failed to save spreadsheet: %w", err)
	}

	fmt.Printf("✓ Summary spreadsheet created: %s\n", filePath)
	return nil
}

func createPricingDetailsSheet(f *excelize.File, result *models.ComparisonResult) {
	f.NewSheet("Pricing Details")

	row := 1
	for _, plan := range result.Plans {
		// Plan header
		f.SetCellValue("Pricing Details", fmt.Sprintf("A%d", row), plan.CompanyName)
		f.SetCellValue("Pricing Details", fmt.Sprintf("B%d", row), plan.PlanName)
		row++

		// Contract info
		f.SetCellValue("Pricing Details", fmt.Sprintf("A%d", row), "Contract:")
		f.SetCellValue("Pricing Details", fmt.Sprintf("B%d", row), fmt.Sprintf("%d months", plan.ContractTermMonths))
		row++

		// Energy charge
		f.SetCellValue("Pricing Details", fmt.Sprintf("A%d", row), "Energy Charge:")
		f.SetCellValue("Pricing Details", fmt.Sprintf("B%d", row), fmt.Sprintf("%.2f ¢/kWh", plan.EnergyCharge.PerKwhCents))
		row++

		// Base charge
		f.SetCellValue("Pricing Details", fmt.Sprintf("A%d", row), "Base Fee:")
		f.SetCellValue("Pricing Details", fmt.Sprintf("B%d", row), fmt.Sprintf("$%.2f/month", plan.BaseCharge.MonthlyFixed))
		row++

		// Delivery charges
		f.SetCellValue("Pricing Details", fmt.Sprintf("A%d", row), "Delivery Charge (Fixed):")
		f.SetCellValue("Pricing Details", fmt.Sprintf("B%d", row), fmt.Sprintf("$%.2f/month", plan.DeliveryCharge.MonthlyFixed))
		row++

		f.SetCellValue("Pricing Details", fmt.Sprintf("A%d", row), "Delivery Charge (Variable):")
		f.SetCellValue("Pricing Details", fmt.Sprintf("B%d", row), fmt.Sprintf("%.4f ¢/kWh", plan.DeliveryCharge.PerKwhCents))
		row++

		// Time of use rates
		if len(plan.TimeOfUseRates) > 0 {
			f.SetCellValue("Pricing Details", fmt.Sprintf("A%d", row), "Time-of-Use Rates:")
			row++
			for _, tou := range plan.TimeOfUseRates {
				f.SetCellValue("Pricing Details", fmt.Sprintf("A%d", row), fmt.Sprintf("  %s", tou.Name))
				f.SetCellValue("Pricing Details", fmt.Sprintf("B%d", row), fmt.Sprintf("%.2f ¢/kWh", tou.RateCents))
				f.SetCellValue("Pricing Details", fmt.Sprintf("C%d", row), tou.Description)
				row++
			}
		}

		// Observations
		if len(plan.Observations) > 0 {
			f.SetCellValue("Pricing Details", fmt.Sprintf("A%d", row), "Observations:")
			row++
			for _, obs := range plan.Observations {
				f.SetCellValue("Pricing Details", fmt.Sprintf("A%d", row), fmt.Sprintf("  • %s", obs))
				row++
			}
		}

		row += 2 // Spacing between plans
	}

	f.SetColWidth("Pricing Details", "A", "A", 30)
	f.SetColWidth("Pricing Details", "B", "B", 30)
	f.SetColWidth("Pricing Details", "C", "C", 50)
}

func generatePerPlanReports(result *models.ComparisonResult, outputDir string) error {
	for i := range result.Plans {
		planDir := filepath.Join(outputDir, sanitizeName(result.Plans[i].PlanName))
		if err := os.MkdirAll(planDir, 0755); err != nil {
			return fmt.Errorf("failed to create plan directory: %w", err)
		}

		// Generate monthly bills
		if err := generateMonthlyBills(result, &result.Plans[i], planDir); err != nil {
			return fmt.Errorf("failed to generate monthly bills for %s: %w", result.Plans[i].PlanName, err)
		}

		// Generate observations report
		if err := generateObservationsReport(&result.Plans[i], planDir); err != nil {
			return fmt.Errorf("failed to generate observations for %s: %w", result.Plans[i].PlanName, err)
		}
	}

	return nil
}

func generateMonthlyBills(result *models.ComparisonResult, plan *models.Plan, planDir string) error {
	f := excelize.NewFile()
	defer f.Close()

	f.SetSheetName("Sheet1", "Monthly Bills")

	// Headers
	headers := []string{
		"Month",
		"Total Consumption (kWh)",
		"Energy Charge ($)",
		"Base Fee ($)",
		"Delivery Charge ($)",
		"Total Bill ($)",
		"Avg Cost per kWh (¢)",
	}

	for col, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+rune(col))
		f.SetCellValue("Monthly Bills", cell, header)
	}

	// Collect and sort months
	months := []string{}
	if bills, ok := result.Bills[plan.PlanName]; ok {
		for month := range bills {
			months = append(months, month)
		}
		sort.Strings(months)

		// Add bill data
		for row, month := range months {
			bill := bills[month]
			rowNum := row + 2

			avgCost := 0.0
			if bill.TotalConsumptionKwh > 0 {
				avgCost = (bill.TotalBillDollars / bill.TotalConsumptionKwh) * 100
			}

			cells := []interface{}{
				bill.Month.Format("2006-01"),
				fmt.Sprintf("%.2f", bill.TotalConsumptionKwh),
				fmt.Sprintf("%.2f", bill.EnergyChargeDollars),
				fmt.Sprintf("%.2f", bill.BaseChargeDollars),
				fmt.Sprintf("%.2f", bill.DeliveryChargeDollars),
				fmt.Sprintf("%.2f", bill.TotalBillDollars),
				fmt.Sprintf("%.4f", avgCost),
			}

			for col, val := range cells {
				cell := fmt.Sprintf("%c%d", 'A'+rune(col), rowNum)
				f.SetCellValue("Monthly Bills", cell, val)
			}
		}
	}

	f.SetColWidth("Monthly Bills", "A", "A", 15)
	f.SetColWidth("Monthly Bills", "B", "G", 18)

	// Create detailed daily breakdowns
	createDailyBreakdownSheets(f, result, plan)

	// Create calculation details sheet
	createCalculationDetailsSheet(f, result, plan)

	filePath := filepath.Join(planDir, "Simulated_Bills.xlsx")
	if err := f.SaveAs(filePath); err != nil {
		return fmt.Errorf("failed to save bills: %w", err)
	}

	fmt.Printf("✓ Bill simulation created for %s: %s\n", plan.PlanName, filePath)
	return nil
}

func createDailyBreakdownSheets(f *excelize.File, result *models.ComparisonResult, plan *models.Plan) {
	if bills, ok := result.Bills[plan.PlanName]; ok {
		f.NewSheet("Daily Breakdown")

		row := 1
		for _, month := range getSortedMonths(bills) {
			bill := bills[month]

			// Month header
			f.SetCellValue("Daily Breakdown", fmt.Sprintf("A%d", row), bill.Month.Format("January 2006"))
			f.MergeCell("Daily Breakdown", fmt.Sprintf("A%d", row), fmt.Sprintf("D%d", row))
			row++

			// Daily headers
			headers := []string{"Date", "Day of Week", "Consumption (kWh)", "Cost ($)"}
			for col, header := range headers {
				f.SetCellValue("Daily Breakdown", fmt.Sprintf("%c%d", 'A'+rune(col), row), header)
			}
			row++

			// Daily data
			for _, daily := range bill.DailyBreakdowns {
				f.SetCellValue("Daily Breakdown", fmt.Sprintf("A%d", row), daily.Date.Format("2006-01-02"))
				f.SetCellValue("Daily Breakdown", fmt.Sprintf("B%d", row), daily.DayOfWeek)
				f.SetCellValue("Daily Breakdown", fmt.Sprintf("C%d", row), fmt.Sprintf("%.2f", daily.ConsumptionKwh))
				f.SetCellValue("Daily Breakdown", fmt.Sprintf("D%d", row), fmt.Sprintf("%.2f", daily.CostDollars))
				row++
			}

			row += 2
		}

		f.SetColWidth("Daily Breakdown", "A", "A", 15)
		f.SetColWidth("Daily Breakdown", "B", "D", 18)
	}
}

func createCalculationDetailsSheet(f *excelize.File, result *models.ComparisonResult, plan *models.Plan) {
	if bills, ok := result.Bills[plan.PlanName]; ok && len(bills) > 0 {

		f.NewSheet("Calculation Method")

		row := 1
		f.SetCellValue("Calculation Method", "A1", "How Costs Were Calculated")
		row += 2

		f.SetCellValue("Calculation Method", fmt.Sprintf("A%d", row), "Plan Details:")
		row++
		f.SetCellValue("Calculation Method", fmt.Sprintf("A%d", row), fmt.Sprintf("Company: %s", plan.CompanyName))
		row++
		f.SetCellValue("Calculation Method", fmt.Sprintf("A%d", row), fmt.Sprintf("Plan: %s", plan.PlanName))
		row++
		f.SetCellValue("Calculation Method", fmt.Sprintf("A%d", row), fmt.Sprintf("Product Type: %s", plan.ProductType))
		row += 2

		f.SetCellValue("Calculation Method", fmt.Sprintf("A%d", row), "Rate Components:")
		row++
		f.SetCellValue("Calculation Method", fmt.Sprintf("A%d", row), fmt.Sprintf("Energy Rate: %.4f ¢/kWh = $%.6f/kWh", plan.EnergyCharge.PerKwhCents, plan.EnergyCharge.PerKwhDollars))
		row++
		f.SetCellValue("Calculation Method", fmt.Sprintf("A%d", row), fmt.Sprintf("Base Fee: $%.2f/month", plan.BaseCharge.MonthlyFixed))
		row++
		f.SetCellValue("Calculation Method", fmt.Sprintf("A%d", row), fmt.Sprintf("Delivery Fixed: $%.2f/month", plan.DeliveryCharge.MonthlyFixed))
		row++
		f.SetCellValue("Calculation Method", fmt.Sprintf("A%d", row), fmt.Sprintf("Delivery Variable: %.4f ¢/kWh = $%.6f/kWh", plan.DeliveryCharge.PerKwhCents, plan.DeliveryCharge.PerKwhDollars))
		row += 2

		if plan.ProductType == "Time Of Use" && len(plan.TimeOfUseRates) > 0 {
			f.SetCellValue("Calculation Method", fmt.Sprintf("A%d", row), "Time-of-Use Periods:")
			row++
			for _, tou := range plan.TimeOfUseRates {
				f.SetCellValue("Calculation Method", fmt.Sprintf("A%d", row), fmt.Sprintf("%s: %.2f ¢/kWh - %s", tou.Name, tou.RateCents, tou.Description))
				row++
			}
		}

		f.SetColWidth("Calculation Method", "A", "A", 80)
	}
}

func generateObservationsReport(plan *models.Plan, planDir string) error {
	// Create a text file with observations
	obsPath := filepath.Join(planDir, "Plan_Observations.txt")

	content := fmt.Sprintf(`Plan Observations Report
=========================

Company: %s
Plan Name: %s
Product Type: %s
Contract Term: %d months
PUCT Certificate: %s

Plan Details:
- Energy Charge: %.4f ¢/kWh
- Base Monthly Fee: $%.2f
- Delivery Fixed: $%.2f/month
- Delivery Variable: %.4f ¢/kWh
- Renewable Content: %.1f%%
- Termination Fee: $%.2f

Key Observations:
`,
		plan.CompanyName,
		plan.PlanName,
		plan.ProductType,
		plan.ContractTermMonths,
		plan.PUCTCertificate,
		plan.EnergyCharge.PerKwhCents,
		plan.BaseCharge.MonthlyFixed,
		plan.DeliveryCharge.MonthlyFixed,
		plan.DeliveryCharge.PerKwhCents,
		plan.RenewablePercent,
		plan.TerminationFeeDollars,
	)

	if len(plan.Observations) > 0 {
		for i, obs := range plan.Observations {
			content += fmt.Sprintf("%d. %s\n", i+1, obs)
		}
	} else {
		content += "- No special observations\n"
	}

	if len(plan.TimeOfUseRates) > 0 {
		content += "\nTime-of-Use Rate Details:\n"
		for _, tou := range plan.TimeOfUseRates {
			content += fmt.Sprintf("- %s: %.2f ¢/kWh (%s, %s)\n  %s\n",
				tou.Name, tou.RateCents, tou.DayOfWeek,
				fmt.Sprintf("%02d:00-%02d:00", tou.StartHour, tou.EndHour),
				tou.Description)
		}
	}

	if err := os.WriteFile(obsPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write observations: %w", err)
	}

	fmt.Printf("✓ Observations report created for %s: %s\n", plan.PlanName, obsPath)
	return nil
}

func getSortedMonths(bills map[string]models.MonthBill) []string {
	months := make([]string, 0, len(bills))
	for month := range bills {
		months = append(months, month)
	}
	sort.Strings(months)
	return months
}

func sanitizeName(name string) string {
	// Replace invalid filename characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(name)
}
