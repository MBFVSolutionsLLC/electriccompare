package parser

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"electriccompare/internal/models"
)

// ParsePDF extracts pricing information from an Electricity Facts Label PDF
func ParsePDF(filePath string) (*models.Plan, error) {
	// Read the PDF file as binary and extract readable strings
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF file: %w", err)
	}

	rawText := extractPDFText(data)
	if len(rawText) < 50 {
		rawText = extractReadableStrings(string(data))
	}

	if len(rawText) < 50 {
		return nil, fmt.Errorf("could not extract sufficient text from PDF")
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

	if plan.EnergyCharge.PerKwhCents == 0 &&
		plan.DeliveryCharge.PerKwhCents == 0 &&
		plan.DeliveryCharge.MonthlyFixed == 0 &&
		plan.BaseCharge.MonthlyFixed == 0 {
		return nil, fmt.Errorf("no electricity pricing components found")
	}

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

	if planName := findPlanNameNearIssueDate(text); planName != "" {
		plan.PlanName = planName
	} else {
		planPatterns := []string{
			`(?is)CleanSky Energy\s+([^\n]+?)\s+Issue Date`,
			`(?im)^Plan\s*Name[:\s]+([^\n]+)`,
			`(?im)^([A-Za-z][A-Za-z0-9\s-]+(?:Fixed|Time Of Use|Variable))\s*$`,
		}

		for _, pattern := range planPatterns {
			re := regexp.MustCompile(pattern)
			if matches := re.FindStringSubmatch(text); len(matches) > 1 {
				plan.PlanName = cleanTextValue(matches[1])
				if isLikelyPlanName(plan.PlanName) {
					break
				}
				plan.PlanName = ""
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

	productTypeRe := regexp.MustCompile(`(?i)Type\s+of\s+Product\s+([^\n]+)`)
	if matches := productTypeRe.FindStringSubmatch(text); len(matches) > 1 {
		plan.ProductType = cleanTextValue(matches[1])
	}
	if strings.Contains(strings.ToLower(plan.ProductType), "time of use") || strings.Contains(text, "Time Of Use") {
		plan.ProductType = "Time Of Use"
	} else if strings.Contains(strings.ToLower(plan.ProductType), "fixed") || strings.Contains(text, "Fixed Rate") || strings.Contains(text, "Fixed") {
		plan.ProductType = "Fixed"
	} else if strings.Contains(strings.ToLower(plan.ProductType), "variable") || strings.Contains(text, "Variable") {
		plan.ProductType = "Variable"
	}
}

func parsePricing(text string, plan *models.Plan) {
	// Energy charge - look for patterns like "13.1 ¢ per kWh" or "13.1 cents per kWh"
	energyPatterns := []string{
		`(?is)Energy\s*Charge[:\s]+([0-9.]+)\s*(?:¢|¡|cents?|c).*?(?:per|\/)\s*kWh`,
		`(?is)(?:Paid|energy).*?Period[:\s]+([0-9.]+)\s*(?:¢|¡|cents?|c).*?(?:per|\/)\s*kWh`,
		`(?is)(\d+\.?\d*)\s*(?:¢|¡|cents?|c).*?kWh.*?energy`,
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
		`(?is)Base\s*(?:Charge|Fee)[:\s]+\$?([0-9.]+)`,
		`(?is)(?:no monthly|monthly)\s*(?:charge|fee)[:\s]*\$?([0-9.]*)`,
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
		`(?is)(?:Oncor|TDSP|Delivery).*?Charges[:\s]+\$([0-9.]+).*?month.*?([0-9.]+)\s*(?:¢|¡|cents?|c)`,
		`(?is)(\$[\d.]+)\s*(?:per month|monthly).*?and\s+([0-9.]+)\s*(?:¢|¡|cents?|c)`,
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
		`(?is)Designated\s*Free\s*(?:Energy|Period)[:\s]+([^\n]+(?:\n[^\n]+)?)`,
		`(?is)Free.*?from\s+([0-9:]+\s*(?:am|pm)?)\s*(?:to|-|through)\s+([0-9:]+\s*(?:am|pm)?)(?:\s+on\s+)?([^\n]+)`,
	}

	for _, pattern := range freePatterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(text); len(matches) > 0 {
			desc := cleanTextValue(matches[1])

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
			timeRe := regexp.MustCompile(`(?i)(\d{1,2})(?::(\d{2}))?\s*(am|pm)?`)
			if matches := timeRe.FindAllStringSubmatch(desc, -1); len(matches) >= 2 {
				tou.StartHour = parseClockHour(matches[0][1], matches[0][3])
				tou.EndHour = parseClockHour(matches[len(matches)-1][1], matches[len(matches)-1][3])
			}

			plan.TimeOfUseRates = append(plan.TimeOfUseRates, tou)
			break
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
		`(?is)(?:Termination|Cancellation)\s*Fee[:\s-]+\$([0-9.]+)`,
		`(?is)Fee\s*for\s*(?:Early)?.*?(?:Termination|Cancellation)[:\s]+\$([0-9.]+)`,
		`(?is)terminating\s+service\?\s*\$([0-9.]+)`,
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

func extractPDFText(data []byte) string {
	streams := decompressPDFStreams(data)
	var parts []string
	for _, stream := range streams {
		parts = append(parts, extractPDFTextOperators(stream)...)
	}
	return normalizeExtractedText(parts)
}

func decompressPDFStreams(data []byte) [][]byte {
	streamRe := regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	matches := streamRe.FindAllSubmatch(data, -1)
	streams := make([][]byte, 0, len(matches))
	for _, match := range matches {
		payload := bytes.Trim(match[1], "\r\n")
		reader, err := zlib.NewReader(bytes.NewReader(payload))
		if err != nil {
			continue
		}
		decoded, err := io.ReadAll(reader)
		reader.Close()
		if err == nil {
			streams = append(streams, decoded)
		}
	}
	return streams
}

func extractPDFTextOperators(stream []byte) []string {
	var parts []string

	tjRe := regexp.MustCompile(`(?s)\((?:\\.|[^\\)])*\)\s*Tj`)
	for _, match := range tjRe.FindAll(stream, -1) {
		closeParen := bytes.LastIndexByte(match, ')')
		if closeParen <= 0 {
			continue
		}
		decoded := decodePDFLiteralString(match[1:closeParen])
		if strings.TrimSpace(decoded) != "" {
			parts = append(parts, decoded)
		}
	}

	arrayTJRe := regexp.MustCompile(`(?s)\[(.*?)\]\s*TJ`)
	stringRe := regexp.MustCompile(`(?s)\((?:\\.|[^\\)])*\)`)
	for _, match := range arrayTJRe.FindAllSubmatch(stream, -1) {
		var b strings.Builder
		for _, str := range stringRe.FindAll(match[1], -1) {
			closeParen := bytes.LastIndexByte(str, ')')
			if closeParen <= 0 {
				continue
			}
			b.WriteString(decodePDFLiteralString(str[1:closeParen]))
		}
		if text := strings.TrimSpace(b.String()); text != "" {
			parts = append(parts, text)
		}
	}

	return parts
}

func decodePDFLiteralString(raw []byte) string {
	unescaped := unescapePDFLiteral(raw)
	if len(unescaped) >= 2 && len(unescaped)%2 == 0 {
		var b strings.Builder
		for i := 0; i+1 < len(unescaped); i += 2 {
			code := int(unescaped[i])<<8 | int(unescaped[i+1])
			switch {
			case code == 0x0003:
				b.WriteByte(' ')
			case code >= 0x0003 && code <= 0x00b4:
				b.WriteRune(rune(code + 29))
			case code >= 32 && code <= 126:
				b.WriteByte(byte(code))
			}
		}
		if text := b.String(); strings.TrimSpace(text) != "" {
			return text
		}
	}
	return string(unescaped)
}

func unescapePDFLiteral(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			out = append(out, raw[i])
			continue
		}
		i++
		if i >= len(raw) {
			break
		}
		switch raw[i] {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case '(', ')', '\\':
			out = append(out, raw[i])
		case '\n':
		case '\r':
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
		default:
			if raw[i] >= '0' && raw[i] <= '7' {
				octal := []byte{raw[i]}
				for j := 0; j < 2 && i+1 < len(raw) && raw[i+1] >= '0' && raw[i+1] <= '7'; j++ {
					i++
					octal = append(octal, raw[i])
				}
				if val, err := strconv.ParseInt(string(octal), 8, 16); err == nil {
					out = append(out, byte(val))
				}
			} else {
				out = append(out, raw[i])
			}
		}
	}
	return out
}

func normalizeExtractedText(parts []string) string {
	var lines []string
	for _, part := range parts {
		for _, line := range strings.Split(part, "\n") {
			line = cleanTextValue(line)
			if line != "" {
				lines = append(lines, line)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func cleanTextValue(value string) string {
	value = strings.ReplaceAll(value, "¡", "¢")
	value = strings.ReplaceAll(value, "Î", "-")
	value = strings.ReplaceAll(value, "\u0086", "")
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}

func parseClockHour(hourText, suffix string) int {
	hour, err := strconv.Atoi(hourText)
	if err != nil {
		return 0
	}
	suffix = strings.ToLower(suffix)
	if suffix == "pm" && hour != 12 {
		hour += 12
	}
	if suffix == "am" && hour == 12 {
		return 0
	}
	return hour
}

func findPlanNameNearIssueDate(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.Contains(strings.ToLower(line), "issue date") {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			candidate := cleanTextValue(lines[j])
			if isLikelyPlanName(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func isLikelyPlanName(value string) bool {
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	rejected := []string{
		"type of product",
		"contract term",
		"electricity facts label",
		"cleansky energy",
		"oncor service area",
		"renewable content",
	}
	for _, reject := range rejected {
		if strings.Contains(lower, reject) {
			return false
		}
	}
	return strings.Contains(lower, "fixed") ||
		strings.Contains(lower, "time of use") ||
		strings.Contains(lower, "variable")
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
		"1H":    1,
		"1DAY":  2,
		"15MIN": 3,
		"1MIN":  4,
		"1SEC":  5,
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
func extractTextFromPage(p interface{}) (string, error) {
	// This function is no longer needed but keeping for compatibility
	return "", nil
}

// getPageText extracts text from a PDF page
func getPageText(pageNum int) (string, error) {
	// This function is no longer needed but keeping for compatibility
	return "", nil
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
