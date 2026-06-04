package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"electriccompare/internal/calculator"
	"electriccompare/internal/models"
	"electriccompare/internal/output"
	"electriccompare/internal/parser"
)

func main() {
	factsDir := flag.String("facts", "", "Directory containing Electricity Facts Label PDF files")
	consumptionDir := flag.String("consumption", "", "Directory containing consumption CSV files")
	outputDir := flag.String("output", "./electricompare_output", "Output directory for reports")
	flag.Parse()

	if *factsDir == "" || *consumptionDir == "" {
		fmt.Println("ElectricCompare - Electricity Plan Comparison Tool")
		fmt.Println("=====================================================")
		fmt.Println("\nUsage: electriccompare -facts <pdf_dir> -consumption <csv_dir> [-output <output_dir>]")
		fmt.Println("\nFlags:")
		fmt.Println("  -facts         Directory containing Electricity Facts Label PDF files")
		fmt.Println("  -consumption   Directory containing consumption CSV files")
		fmt.Println("  -output        Output directory (default: ./electricompare_output)")
		fmt.Println("\nExample:")
		fmt.Println("  electriccompare -facts ./facts -consumption ./comsuption -output ./results")
		os.Exit(1)
	}

	// Validate directories
	if err := validateDirectory(*factsDir); err != nil {
		fmt.Printf("Error: Facts directory - %v\n", err)
		os.Exit(1)
	}
	if err := validateDirectory(*consumptionDir); err != nil {
		fmt.Printf("Error: Consumption directory - %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("ElectricCompare - Electricity Plan Comparison Tool")
	fmt.Println("=====================================================")
	fmt.Println()

	result := &models.ComparisonResult{
		Plans:        []models.Plan{},
		Consumptions: make(map[string][]models.ConsumptionRecord),
		Bills:        make(map[string]map[string]models.MonthBill),
		Observations: []string{},
	}

	// Parse PDFs
	fmt.Println("Parsing Electricity Facts PDFs...")
	if err := parsePDFs(*factsDir, result); err != nil {
		fmt.Printf("Error parsing PDFs: %v\n", err)
		os.Exit(1)
	}

	// Parse consumption data
	fmt.Println("\nParsing consumption CSV files...")
	if err := parseConsumptionData(*consumptionDir, result); err != nil {
		fmt.Printf("Error parsing consumption data: %v\n", err)
		os.Exit(1)
	}

	// Calculate bills
	fmt.Println("\nCalculating bills for each plan...")
	calculateAllBills(result)

	// Generate output
	fmt.Println("\nGenerating output reports...")
	if err := output.GenerateOutput(result, *outputDir); err != nil {
		fmt.Printf("Error generating output: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	printSummary(result, *outputDir)
}

func validateDirectory(dir string) error {
	stat, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("not a directory: %s", dir)
	}
	return nil
}

func parsePDFs(factsDir string, result *models.ComparisonResult) error {
	entries, err := os.ReadDir(factsDir)
	if err != nil {
		return err
	}

	pdfCount := 0
	planNameCounts := make(map[string]int)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".pdf" {
			continue
		}

		filePath := filepath.Join(factsDir, entry.Name())
		fmt.Printf("  Parsing: %s... ", entry.Name())

		plan, err := parser.ParsePDF(filePath)
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			continue
		}

		plan.PlanName = uniquePlanName(plan.PlanName, entry.Name(), planNameCounts)
		result.Plans = append(result.Plans, *plan)
		fmt.Printf("✓ %s\n", plan.PlanName)
		pdfCount++
	}

	if pdfCount == 0 {
		return fmt.Errorf("no PDF files found in %s", factsDir)
	}

	fmt.Printf("\n✓ Parsed %d plans\n", pdfCount)
	return nil
}

func parseConsumptionData(consumptionDir string, result *models.ComparisonResult) error {
	filePath, err := parser.FindConsumptionFile(consumptionDir, "1H")
	if err != nil {
		return err
	}

	fmt.Printf("  Reading: %s... ", filepath.Base(filePath))
	records, err := parser.ParseConsumptionCSV(filePath)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		return err
	}

	result.Consumptions[filepath.Base(filePath)] = records
	fmt.Printf("✓ %d records\n", len(records))

	fmt.Printf("✓ Selected %s for bill simulation\n", filepath.Base(filePath))
	return nil
}

func calculateAllBills(result *models.ComparisonResult) {
	for i := range result.Plans {
		planName := result.Plans[i].PlanName
		result.Bills[planName] = make(map[string]models.MonthBill)

		// Use all consumption data
		for _, records := range result.Consumptions {
			bills := calculator.CalculateBills(&result.Plans[i], records)
			for monthKey, bill := range bills {
				result.Bills[planName][monthKey] = bill
			}
		}

		fmt.Printf("  ✓ Calculated bills for: %s (%d months)\n", planName, len(result.Bills[planName]))
	}
}

func printSummary(result *models.ComparisonResult, outputDir string) {
	fmt.Println("\n=====================================================")
	fmt.Println("Analysis Summary")
	fmt.Println("=====================================================")
	fmt.Println()

	fmt.Printf("Plans Analyzed: %d\n", len(result.Plans))
	fmt.Printf("Consumption Files: %d\n", len(result.Consumptions))

	// Show monthly bill ranges
	fmt.Println("\nMonthly Bill Ranges by Plan:")
	for _, plan := range result.Plans {
		if bills, ok := result.Bills[plan.PlanName]; ok {
			if len(bills) > 0 {
				// Find min/max
				minBill := 999999.0
				maxBill := 0.0
				totalBill := 0.0
				monthCount := 0

				for _, bill := range bills {
					if bill.TotalBillDollars < minBill {
						minBill = bill.TotalBillDollars
					}
					if bill.TotalBillDollars > maxBill {
						maxBill = bill.TotalBillDollars
					}
					totalBill += bill.TotalBillDollars
					monthCount++
				}

				avgBill := totalBill / float64(monthCount)
				fmt.Printf("  %s:\n", plan.PlanName)
				fmt.Printf("    Range: $%.2f - $%.2f\n", minBill, maxBill)
				fmt.Printf("    Average: $%.2f/month\n", avgBill)
				fmt.Printf("    Total (all months): $%.2f\n\n", totalBill)
			}
		}
	}

	fmt.Printf("Output Location: %s\n\n", outputDir)
	fmt.Println("✓ Analysis complete! Check the output directory for detailed reports.")
}

func uniquePlanName(planName, fileName string, counts map[string]int) string {
	planName = strings.TrimSpace(planName)
	if planName == "" {
		planName = strings.TrimSuffix(fileName, filepath.Ext(fileName))
	}

	counts[planName]++
	if counts[planName] == 1 {
		return planName
	}
	return fmt.Sprintf("%s (%d)", planName, counts[planName])
}
