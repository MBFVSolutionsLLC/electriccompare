# ElectricCompare - Setup and Usage Guide

## Overview

ElectricCompare is a Go command-line application that analyzes electricity pricing plans and simulates bills based on your consumption patterns. It parses Electricity Facts Label PDFs, reads consumption data from CSV files, and generates comprehensive billing simulations.

## What's Included

### Executable
- `cmd/electriccompare/electriccompare.exe` - The compiled Go application

### Source Code
- `internal/models/` - Data structures for plans, consumption, bills
- `internal/parser/` - PDF and CSV parsing
- `internal/calculator/` - Bill calculation engine
- `internal/output/` - Excel and report generation
- `cmd/electriccompare/` - CLI application

## Quick Start

### 1. Run the Analysis
```bash
cd c:\GitHub\mbfvsolutionsllc\electriccompare

# Basic usage
.\cmd\electriccompare\electriccompare.exe -facts .\facts -consumption .\comsumption -output .\results

# Custom output directory
.\cmd\electriccompare\electriccompare.exe `
  -facts C:\path\to\pdfs `
  -consumption C:\path\to\csvs `
  -output C:\path\to\results
```

### 2. Check the Output
The `results/` directory will contain:
- **Plan_Comparison_Summary.xlsx** - Overview of all plans with pricing details
- **[Plan Name]/** - Directory per plan containing:
  - `Simulated_Bills.xlsx` - Monthly bills with daily breakdowns
  - `Plan_Observations.txt` - Plan details and special terms

## Input File Requirements

### Consumption CSV Files
Must be in this format:
```
Time Bucket (America/Chicago),Vue-Mains_A (kWhs),[other appliance columns...]
12/04/2025 00:00:00,4.9120,[other values...]
12/04/2025 01:00:00,1.6428,[other values...]
```

Supported time granularities (in order of preference):
- `1H.csv` - Hourly data ✓ Best for most plans
- `1DAY.csv` - Daily aggregated
- `15MIN.csv` - 15-minute intervals
- `1MIN.csv` - Minute-level data
- `1SEC.csv` - Second-level data

The app automatically selects the most appropriate file. The **first column labeled "Vue-Mains_A"** contains total consumption in kWh.

### Electricity Facts Label PDFs
Standard Texas PUCT format documents containing:
- Plan and company information
- Pricing rates and fees
- Time-of-use periods (if applicable)
- Contract terms

**Important:** PDFs must be **text-based** (not scanned images). If your PDFs are scanned, use OCR first to convert them to searchable PDFs.

## PDF Text Extraction Issues

The application uses binary string extraction from PDFs as a fallback method. For better results with complex PDF structures, consider:

### Option 1: Use pdfplumber (Recommended)
```bash
pip install pdfplumber

# Create Python script to extract text
python extract_pdfs.py ./facts ./pdf_text_output

# Then review the text files to verify extraction
cat pdf_text_output/Electricity_Facts_Label_V1.04.txt
```

### Option 2: Online PDF Converter
- Upload PDFs to an online converter (e.g., ILovePDF, Smallpdf)
- Convert to searchable PDF or export as text
- Use the converted files with ElectricCompare

### Option 3: Manual Review
If PDF parsing produces $0.00 bills:
1. Open the plan directories in results/
2. Edit `Plan_Observations.txt` with the correct pricing details
3. The raw extracted text is in Plan_Comparison_Summary.xlsx

## Output File Details

### Plan_Comparison_Summary.xlsx
Two sheets:
1. **Plan Summary** - Quick reference table with:
   - Company name and plan name
   - Product type (Fixed, Time-of-Use, Variable)
   - Energy charges (¢/kWh)
   - Base fees, delivery charges
   - Contract terms and termination fees
   
2. **Pricing Details** - Detailed breakdown of each plan

### [Plan Name]/Simulated_Bills.xlsx
Four sheets:
1. **Monthly Bills** - Summary by month
   - Total consumption (kWh)
   - Energy charge ($)
   - Base fee ($)
   - Delivery charge ($)
   - Total bill ($)
   - Average cost per kWh (¢)

2. **Daily Breakdown** - Day-by-day analysis
   - Consumption and cost per day
   - Day of week
   - Rates applied

3. **Calculation Method** - How costs were calculated
   - Rate components
   - Formulas used
   - Time-of-use periods (if applicable)

### [Plan Name]/Plan_Observations.txt
Plain text file containing:
- Plan identification
- Extracted pricing components
- Special terms and observations
- Time-of-use rate details

## Bill Calculation Method

### Fixed Rate Plans
```
Monthly Bill = Base Fee + (Total kWh × Energy Rate) + 
               (Delivery Fixed + Total kWh × Delivery Rate)
```

### Time-of-Use Plans
```
For each hour:
  Determine rate based on:
    - Hour of day (e.g., 3pm-6pm)
    - Day of week (weekday vs. weekend)
    - Period (free vs. paid)
  
  Cost = Consumption × Applicable Rate

Total Bill = Sum of all hourly costs + Base Fee + Delivery Charge
```

## Troubleshooting

### "Error: no PDF files found"
- Verify PDFs are in the specified directory
- Check file extensions are `.pdf` (lowercase)
- Ensure you have read permissions

### "Error: no consumption CSV found"
- CSV files must contain "Vue-Mains_A" column
- Check filename includes time granularity (e.g., Vue-1H.csv)
- Verify column headers match exactly

### Bills showing $0.00
- PDF text extraction may have failed
- Check Plan_Observations.txt for extracted content
- If empty, PDF may be scanned or corrupted
- See "PDF Text Extraction Issues" above

### Plan names empty or cut off
- Check the `Plan_Observations.txt` file in each plan directory
- Manually verify pricing information extracted
- Consider using pdfplumber for better extraction

## Example Workflow

```bash
# 1. Place files in directories
# facts/          - Your Electricity Facts Label PDFs
# comsumption/    - Your consumption CSV files

# 2. Run analysis
.\cmd\electriccompare\electriccompare.exe -facts .\facts -consumption .\comsumption -output .\results

# 3. Open results
# - Open Plan_Comparison_Summary.xlsx to see all plans
# - Open each plan's Simulated_Bills.xlsx to see monthly costs
# - Check Plan_Observations.txt to verify pricing details

# 4. Make decisions
# - Compare monthly bills across plans
# - Consider contract terms and flexibility
# - Check renewable energy content
# - Review any special observations
```

## Technical Details

### Supported Platforms
- Windows ✓ (Tested)
- Linux (Should work)
- macOS (Should work)

### Requirements
- No external dependencies (portable executable)
- Excel 2007+ for opening .xlsx files

### How It Works
1. **PDF Parsing**: Extracts readable text from PDF binary data
2. **CSV Parsing**: Reads consumption records with timezone support
3. **Matching**: Aligns consumption granularity with plan rates
4. **Calculation**: Applies rates and generates monthly bills
5. **Output**: Creates Excel files and text reports

## Limitations

- PDF parsing works best with text-based (not scanned) PDFs
- Assumes America/Chicago timezone for consumption data
- Time-of-use rates must follow standard formats
- No support for tiered pricing (tier 1, tier 2, etc.)

## Future Enhancements

- OCR support for scanned PDFs
- Additional rate structures (tiered, seasonal)
- Comparison charts and visualizations
- Web interface
- Historical data trend analysis
- Cost comparison rankings

## Support

For issues or questions:
1. Check this guide's troubleshooting section
2. Review output files for clues
3. Verify input file formats
4. Consider using pdfplumber for text extraction

## License

See LICENSE file in repository

## Building from Source

```bash
# Prerequisites: Go 1.21+

cd c:\GitHub\mbfvsolutionsllc\electriccompare

# Download dependencies
go mod tidy

# Build executable
go build -o cmd\electriccompare\electriccompare.exe .\cmd\electriccompare

# Run
.\cmd\electriccompare\electriccompare.exe -facts .\facts -consumption .\comsumption
```
