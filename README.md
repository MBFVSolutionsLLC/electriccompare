# ElectricCompare

ElectricCompare is a Go command-line application for comparing electricity plans against real consumption data. It reads Electricity Facts Label PDFs, reads Vue energy consumption CSV files, simulates monthly bills for each plan, and writes Excel/text reports that make the plans easier to compare.

## Project Layout

The runnable Go application lives in `src`.

```text
src/
  main.go                    CLI entrypoint and application workflow
  go.mod                     Go module definition
  internal/
    models/                  Shared data structures
    parser/                  PDF and CSV parsers
    calculator/              Bill calculation engine
    output/                  Excel and text report generation

facts/                       Sample Electricity Facts Label PDFs
comsumption/                 Sample Vue consumption CSV files
results/                     Example generated reports
results_new/                 Example generated reports
```

## How The Code Works

`src/main.go` is the application entrypoint. It accepts three flags:

- `-facts`: directory containing Electricity Facts Label PDF files
- `-consumption`: directory containing consumption CSV files
- `-output`: directory where generated reports are written

The runtime flow is:

1. `main.go` validates the `-facts` and `-consumption` directories.
2. `parsePDFs` scans the facts directory for `.pdf` files and calls `parser.ParsePDF` for each one.
3. `parser.ParsePDF` extracts readable text from each PDF and fills a `models.Plan` with plan metadata, pricing, time-of-use periods, fees, terms, and observations.
4. `parseConsumptionData` scans the consumption directory for `.csv` files and calls `parser.ParseConsumptionCSV`.
5. `parser.ParseConsumptionCSV` reads timestamped kWh records from the `Vue-Mains_A` column.
6. `calculateAllBills` calls `calculator.CalculateBills` for each parsed plan and each consumption dataset.
7. `calculator.CalculateBills` groups usage by month/day/hour, applies fixed or time-of-use rates, adds base and delivery charges, and returns monthly bill summaries.
8. `output.GenerateOutput` writes the comparison files to the output directory.

The main data types are defined in `src/internal/models/models.go`:

- `Plan`: one electricity plan and its parsed pricing details
- `ConsumptionRecord`: one timestamped kWh reading
- `MonthBill`: calculated monthly bill and cost breakdown
- `ComparisonResult`: all parsed plans, consumption data, bills, and observations

## Inputs

### Electricity Facts Label PDFs

Put PDF plan documents in a directory such as `facts/`.

The parser expects text-based PDFs. Scanned image PDFs may not parse correctly unless they are OCR'd first.

### Consumption CSV Files

Put Vue CSV exports in a directory such as `comsumption/`.

The CSV header must include a `Vue-Mains_A` column, which is treated as total household kWh usage. The first column is parsed as the timestamp.

Example:

```csv
Time Bucket (America/Chicago),Vue-Mains_A (kWhs)
12/04/2025 00:00:00,4.9120
12/04/2025 01:00:00,1.6428
```

## Outputs

The output directory contains:

- `Plan_Comparison_Summary.xlsx`: high-level comparison of all parsed plans
- One folder per plan, named from the plan name
- `Simulated_Bills.xlsx`: monthly bills, daily breakdowns, and calculation details for that plan
- `Plan_Observations.txt`: parsed plan details, special notes, and time-of-use information

## Requirements

- Go 1.21 or newer
- Dependencies from `src/go.mod`

The main Go dependencies are:

- `github.com/xuri/excelize/v2` for creating `.xlsx` reports
- `github.com/pdfcpu/pdfcpu` is listed in the module, though the current parser extracts readable strings directly from PDF bytes

## How To Run

From the repository root:

```powershell
cd .\src
go run . -facts ..\facts -consumption ..\comsumption -output ..\results_new
```

You can also pass custom directories:

```powershell
cd .\src
go run . -facts C:\path\to\facts -consumption C:\path\to\csvs -output C:\path\to\results
```

If you omit required flags, the CLI prints usage help:

```powershell
go run .
```

## How To Build

From the repository root:

```powershell
cd .\src
go build -o electriccompare.exe .
```

Run the built executable:

```powershell
.\electriccompare.exe -facts ..\facts -consumption ..\comsumption -output ..\results_new
```

## Notes And Limitations

- The sample consumption folder is named `comsumption` in this repository.
- PDF parsing is heuristic and works best with text-based PDFs in common Electricity Facts Label formats.
- Time-of-use calculations are based on parsed day/hour windows.
- Delivery fixed charges are spread across daily breakdowns as `monthly fixed / 30`, then included in monthly totals.
- If parsed rates are missing or bills show unexpected zero values, inspect the generated `Plan_Observations.txt` files and the source PDFs.
