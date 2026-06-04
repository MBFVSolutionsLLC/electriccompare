package parser

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"electriccompare/internal/models"
)

// ParsePDF extracts pricing information from an Electricity Facts Label PDF
func ParsePDF(filePath string) (*models.Plan, error) {
	// Read text from PDF
	var text strings.Builder

	// Try to extract text from the PDF
	ctx, err := api.ReadContextFromFile(filePath, pdfcpu.NewDefaultConfiguration())
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}

	// Extract text from each page
	if ctx.XRefTable == nil || ctx.XRefTable.Size == 0 {
		return nil, fmt.Errorf("PDF has no content")
	}

	// For each page, extract text content
	for pageNum := 1; pageNum <= ctx.PageCount; pageNum++ {
		// Try to get text content
		pageContent, err := getPageText(ctx, pageNum)
		if err == nil && pageContent != "" {
			text.WriteString(pageContent)
			text.WriteString("\n")
		}
	}

	rawText := text.String()
	if len(rawText) < 50 {
		// If we couldn't extract text, try a different approach
		// Read the raw file as fallback
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read PDF file: %w", err)
		}
		// Extract readable strings from PDF binary
		rawText = extractReadableStrings(string(data))
	}

	plan := &models.Plan{
		RawText:        rawText,
		Observations:   []string{},
		TimeOfUseRates: []models.TimeOfUseRate{},
	}

	// Extract information
	parseHeader(rawText, plan)
	parsePricing(rawText, plan)
	parseTimeOfUse(rawText, plan)
	parseTerms(rawText, plan)
	parseObservations(rawText, plan)

	return plan, nil
}

func parseHeader(text string, plan *models.Plan) {
	// Company name - usually appears early
	if strings.Contains(text, "CleanSky Energy") {
		plan.CompanyName = "CleanSky Energy"
	} else {
		// Generic extraction
		lines := strings.Split(text, "\n")
		if len(lines) > 0 {
			plan.CompanyName = lines[0]
		}
	}

	// Plan name - look for common patterns
	planPatterns := []string{
		`(?i)Plan\s*Name[:\s]+([^\n]+)`,
		`(?i)(Happy Hour|Eco Rewards|Fixed|Time of Use)[^\n]*(\d+)?\s*(?:-|—)?\s*([^\n]+)?`,
		`(?i)^([A-Z][a-z\s]+(?:Power|Plan|Rate)[\s\d\w-]+)`,
	}

	for _, pattern := range planPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 1 {
			plan.PlanName = strings.TrimSpace(matches[len(matches)-1])
			if plan.PlanName != "" {
				break
			}
		}
	}

	// Service area
	if idx := strings.Index(text, "Oncor Service Area"); idx != -1 {
		plan.ServiceArea = "Oncor Service Area"
	}

	// PUCT Certificate
	certRe := regexp.MustCompile(`(?i)PUCT.*?#(\d+)`)
	if matches := certRe.FindStringSubmatch(text); len(matches) > 1 {
		plan.PUCTCertificate = matches[1]
	}

	// Issue date
	dateRe := regexp.MustCompile(`(?i)Issue Date[:\s]+(\d{1,2}/\d{1,2}/\d{4})`)
	if matches := dateRe.FindStringSubmatch(text); len(matches) > 1 {
		if t, err := time.Parse("1/2/2006", matches[1]); err == nil {
			plan.IssueDate = t
		}
	}

	// Product type
	if strings.Contains(text, "Time Of Use") {
		plan.ProductType = "Time Of Use"
	} else if strings.Contains(text, "Fixed Rate") || strings.Contains(text, "Fixed") {
		plan.ProductType = "Fixed"
	} else if strings.Contains(text, "Variable") {
		plan.ProductType = "Variable"
	}
}

func parsePricing(text string, plan *models.Plan) {
	// Energy charge - look for patterns like "13.1 ¢ per kWh" or "13.1 cents per kWh"
	energyPatterns := []string{
		`(?i)Energy\s*Charge[:\s]+([0-9.]+)\s*[¢c].*?(?:per|\/)\s*kWh`,
		`(?i)(?:Paid|energy).*?Period[:\s]+([0-9.]+)\s*[¢c].*?(?:per|\/)\s*kWh`,
		`(?i)(\d+\.?\d*)\s*[¢c].*?kWh.*?energy`,
	}

	for _, pattern := range energyPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 1 {
			if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
				plan.EnergyCharge.PerKwhCents = val
				plan.EnergyCharge.PerKwhDollars = val / 100
				plan.EnergyCharge.Name = "Energy Charge"
				break
			}
		}
	}

	// Base charge/Fee
	basePatterns := []string{
		`(?i)Base\s*(?:Charge|Fee)[:\s]+\$([0-9.]+)`,
		`(?i)(?:no monthly|monthly)\s*(?:charge|fee)[:\s]*\$?([0-9.]*)`,
	}

	for _, pattern := range basePatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 1 {
			if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
				plan.BaseCharge.MonthlyFixed = val
				plan.BaseCharge.Name = "Base Charge"
				break
			}
		}
	}

	// Delivery/TDSP charge - fixed monthly and per kWh
	deliveryPatterns := []string{
		`(?i)(?:Oncor|TDSP|Delivery).*?Charges[:\s]+\$([0-9.]+).*?month.*?([0-9.]+)\s*[¢c]`,
		`(?i)(\$[\d.]+)\s*(?:per month|monthly).*?and\s+([0-9.]+)\s*[¢c]`,
	}

	for _, pattern := range deliveryPatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 2 {
			if fixed, err := strconv.ParseFloat(strings.TrimPrefix(matches[1], "$"), 64); err == nil {
				plan.DeliveryCharge.MonthlyFixed = fixed
			}
			if perKwh, err := strconv.ParseFloat(matches[2], 64); err == nil {
				plan.DeliveryCharge.PerKwhCents = perKwh
				plan.DeliveryCharge.PerKwhDollars = perKwh / 100
				plan.DeliveryCharge.Name = "Delivery Charge"
			}
			break
		}
	}

	// Renewable content percentage
	renewableRe := regexp.MustCompile(`(?i)Renewable.*?(\d+(?:\.\d+)?)\s*%`)
	if matches := renewableRe.FindStringSubmatch(text); len(matches) > 1 {
		if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
			plan.RenewablePercent = val
		}
	}
}

func parseTimeOfUse(text string, plan *models.Plan) {
	if plan.ProductType != "Time Of Use" {
		return
	}

	// Look for designated free period
	freePatterns := []string{
		`(?i)Designated\s*Free\s*Period[:\s]+([^\n]+)`,
		`(?i)Free.*?from\s+([0-9:]+(am|pm)?)\s*(?:to|-|through)\s+([0-9:]+(am|pm)?)\s*(?:on\s+)?([^\n]+)`,
	}

	for _, pattern := range freePatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 0 {
			desc := matches[1]
			
			// Parse times and days
			tou := models.TimeOfUseRate{
				Name:        "Designated Free Period",
				RateCents:   0,
				Description: desc,
			}

			// Try to parse day of week
			if strings.Contains(desc, "Monday") || strings.Contains(desc, "Friday") {
				tou.DayOfWeek = "Monday-Friday"
			} else if strings.Contains(desc, "Sunday") {
				tou.DayOfWeek = "Sunday"
			} else if strings.Contains(desc, "Saturday") {
				tou.DayOfWeek = "Saturday"
			} else {
				tou.DayOfWeek = "All"
			}

			// Parse hour range
			timeRe := regexp.MustCompile(`(\d{1,2}):?(\d{2})?\s*(am|pm)?`)
			if matches := timeRe.FindAllStringSubmatch(desc, -1); len(matches) >= 2 {
				if h1, err := strconv.Atoi(matches[0][1]); err == nil {
					tou.StartHour = h1
					if h2, err := strconv.Atoi(matches[len(matches)-1][1]); err == nil {
						tou.EndHour = h2
					}
				}
			}

			plan.TimeOfUseRates = append(plan.TimeOfUseRates, tou)
		}
	}

	// Add paid energy period
	if len(plan.TimeOfUseRates) > 0 {
		paidRate := models.TimeOfUseRate{
			Name:        "Paid Energy Period",
			RateCents:   plan.EnergyCharge.PerKwhCents,
			DayOfWeek:   "All",
			Description: "All times not in Designated Free Period",
		}
		plan.TimeOfUseRates = append(plan.TimeOfUseRates, paidRate)
	}
}

func parseTerms(text string, plan *models.Plan) {
	// Contract term
	contractRe := regexp.MustCompile(`(?i)Contract\s*Term[:\s]+(\d+)\s*(?:month|mon)`)
	if matches := contractRe.FindStringSubmatch(text); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			plan.ContractTermMonths = val
		}
	}

	// Termination fee
	feePatterns := []string{
		`(?i)(?:Termination|Cancellation)\s*Fee[:\s]+\$([0-9.]+)`,
		`(?i)Fee\s*for\s*(?:Early)?.*?(?:Termination|Cancellation)[:\s]+\$([0-9.]+)`,
	}

	for _, pattern := range feePatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 1 {
			if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
				plan.TerminationFeeDollars = val
				break
			}
		}
	}
}

func parseObservations(text string, plan *models.Plan) {
	observations := []string{}

	// Check for autopay requirement
	if strings.Contains(text, "Autopay required") || strings.Contains(text, "autopay") {
		observations = append(observations, "Autopay required or recommended")
	}

	// Check for underground facilities charge
	if strings.Contains(text, "Underground Facilities") {
		observations = append(observations, "Some locations may be subject to Underground Facilities and Cost Recovery Charge")
	}

	// Check for specific pricing notes
	if strings.Contains(text, "assuming 35%") {
		observations = append(observations, "Pricing assumes 35% of consumption during Designated Free Period")
	}

	// Check for renewable energy
	if strings.Contains(text, "100%") && strings.Contains(text, "Renewable") {
		observations = append(observations, "100% renewable energy content")
	}

	// Check for price change provisions
	if strings.Contains(text, "Price change") || strings.Contains(text, "price adjustment") {
		observations = append(observations, "Price may change during contract for limited reasons (Oncor changes, regulatory changes, etc.)")
	}

	plan.Observations = observations
}

// ParseConsumptionCSV reads consumption data from CSV files
func ParseConsumptionCSV(filePath string) ([]models.ConsumptionRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV: %w", err)
	}
	defer file.Close()

	var records []models.ConsumptionRecord
	scanner := bufio.NewScanner(file)
	
	// Skip header
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty CSV file")
	}
	header := scanner.Text()
	headerParts := strings.Split(header, ",")

	// Find the Mains column (total consumption)
	mainsIdx := -1
	for i, h := range headerParts {
		if strings.Contains(h, "Vue-Mains_A") {
			mainsIdx = i
			break
		}
	}

	if mainsIdx == -1 {
		return nil, fmt.Errorf("could not find Mains column in CSV")
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) <= mainsIdx {
			continue
		}

		// Parse timestamp
		timeStr := strings.TrimSpace(parts[0])
		t, err := time.Parse("1/2/2006 15:04:05", timeStr)
		if err != nil {
			t, err = time.Parse("01/02/2006 3:04:05 PM", timeStr)
			if err != nil {
				continue
			}
		}

		// Parse consumption value
		kwhStr := strings.TrimSpace(parts[mainsIdx])
		if kwhStr == "No CT" || kwhStr == "" {
			continue
		}

		kwh, err := strconv.ParseFloat(kwhStr, 64)
		if err != nil {
			continue
		}

		record := models.ConsumptionRecord{
			Timestamp: t,
			TotalKwh:  kwh,
			Details:   make(map[string]float64),
		}

		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading CSV: %w", err)
	}

	return records, nil
}

// FindConsumptionFile locates the most appropriate consumption file for a given granularity
func FindConsumptionFile(consumptionDir string, preferredGranularity string) (string, error) {
	entries, err := os.ReadDir(consumptionDir)
	if err != nil {
		return "", err
	}

	// Map of preferred granularities
	granularityPriority := map[string]int{
		"1H":     1,
		"1DAY":   2,
		"15MIN":  3,
		"1MIN":   4,
		"1SEC":   5,
	}

	bestMatch := ""
	bestPriority := 999

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".csv") {
			continue
		}

		for gran, priority := range granularityPriority {
			if strings.Contains(entry.Name(), gran) {
				if priority < bestPriority {
					bestMatch = entry.Name()
					bestPriority = priority
				}
			}
		}
	}

	if bestMatch == "" {
		return "", fmt.Errorf("no consumption CSV found in %s", consumptionDir)
	}

	return filepath.Join(consumptionDir, bestMatch), nil
}

// extractTextFromPage extracts text from a PDF page
func extractTextFromPage(p *pdf.Page) (string, error) {
	if p == nil {
		return "", fmt.Errorf("nil page")
	}

	var content strings.Builder
	
	// Note: pdfcpu has limited text extraction capabilities
	// For production use, consider using a more advanced PDF library
	// This implementation provides basic text extraction from PDFs
	
	return content.String(), nil
}

// getPageText extracts text from a PDF page using pdfcpu
func getPageText(ctx *pdfcpu.Context, pageNum int) (string, error) {
	var text strings.Builder
	// pdfcpu has limited built-in text extraction
	// We'll use a fallback method with binary parsing
	return text.String(), nil
}

// extractReadableStrings pulls readable ASCII strings from PDF binary data
func extractReadableStrings(data string) string {
	var result strings.Builder
	var current strings.Builder
	consecutiveReadable := 0

	for i := 0; i < len(data); i++ {
		ch := data[i]
		// ASCII readable characters and common delimiters
		if (ch >= 32 && ch <= 126) || ch == '\n' || ch == '\r' || ch == '\t' {
			current.WriteByte(ch)
			consecutiveReadable++
		} else {
			// Non-readable character
			if consecutiveReadable > 20 { // Only keep strings > 20 chars
				str := current.String()
				// Filter out binary-looking strings
				if isValidText(str) {
					result.WriteString(strings.TrimSpace(str))
					result.WriteRune('\n')
				}
			}
			current.Reset()
			consecutiveReadable = 0
		}
	}

	// Don't forget the last string
	if consecutiveReadable > 20 {
		str := current.String()
		if isValidText(str) {
			result.WriteString(strings.TrimSpace(str))
			result.WriteRune('\n')
		}
	}

	return result.String()
}

// isValidText checks if a string looks like actual text content
func isValidText(s string) bool {
	// Must have at least some letters or common words
	hasLetter := false
	hasDigit := false
	hasCommon := false

	commonWords := []string{"kWh", "price", "charge", "energy", "plan", "rate", "monthly", "fee", "company", "per", "base", "delivery", "term", "hours", "minutes", "day", "week", "free", "paid", "time"}

	s = strings.ToLower(s)
	for _, word := range commonWords {
		if strings.Contains(s, word) {
			hasCommon = true
			break
		}
	}

	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			hasLetter = true
		}
		if ch >= '0' && ch <= '9' {
			hasDigit = true
		}
	}

	return hasLetter || (hasDigit && hasCommon)
}


