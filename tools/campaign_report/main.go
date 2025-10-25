// Campaign Report Tool generates comprehensive performance reports for ad campaigns.
//
// This tool connects directly to ClickHouse to query analytics data and generates
// formatted reports showing campaign performance metrics, daily breakdowns, and
// creative performance analysis with automated insights.
//
// Usage:
//
//	go run ./tools/campaign_report -campaign-id=123 -days=30
//
// The tool outputs a formatted report including:
//   - Overall performance summary (impressions, clicks, CTR, spend)
//   - Daily performance breakdown
//   - Top performing creatives ranked by CTR
//   - Automated insights and optimization recommendations
//
// Configuration:
//
//	-campaign-id: Required. The campaign ID to generate a report for
//	-days: Optional. Number of days to include in the report (default: 7)
//	-clickhouse-dsn: Optional. ClickHouse connection string (default: tcp://localhost:9000)
//
// Environment Variables:
//
//	CLICKHOUSE_DSN: ClickHouse connection string (overridden by -clickhouse-dsn flag)
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/lib/pq"

	"github.com/patrickwarner/openadserve/internal/reporting"
)

// main is the entry point for the campaign report tool. It parses command line flags,
// establishes a connection to ClickHouse, generates the campaign report, and outputs
// the formatted results to stdout.
func main() {
	var (
		campaignID = flag.Int("campaign-id", 0, "Campaign ID to generate report for")
		days       = flag.Int("days", 7, "Number of days to include in report")
		dsn        = flag.String("clickhouse-dsn", getEnv("CLICKHOUSE_DSN", "tcp://localhost:9000"), "ClickHouse DSN")
	)
	flag.Parse()

	if *campaignID == 0 {
		fmt.Fprintf(os.Stderr, "Error: campaign-id is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Connect to ClickHouse
	db, err := sql.Open("clickhouse", *dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to ClickHouse: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database connection: %v\n", err)
		}
	}()

	if err := db.PingContext(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Error pinging ClickHouse: %v\n", err)
		os.Exit(1)
	}

	// Generate campaign report using shared package
	summary, err := reporting.GenerateCampaignReport(context.Background(), db, *campaignID, *days)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating report: %v\n", err)
		os.Exit(1)
	}

	// Print formatted report
	printCampaignReport(summary, *days)
}

// printCampaignReport outputs a professionally formatted campaign performance report
// to stdout. The report includes overall metrics, daily breakdown tables, creative
// performance analysis, and automated insights with optimization recommendations.
func printCampaignReport(summary *reporting.CampaignSummary, days int) {
	fmt.Printf("═══════════════════════════════════════════════════════════════════════════════════\n")
	fmt.Printf("                              CAMPAIGN PERFORMANCE REPORT                          \n")
	fmt.Printf("═══════════════════════════════════════════════════════════════════════════════════\n")
	fmt.Printf("Campaign ID: %d\n", summary.CampaignID)
	fmt.Printf("Report Period: %d days (ending %s)\n", days, time.Now().Format("2006-01-02"))
	fmt.Printf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	// Overall Performance
	fmt.Printf("📊 OVERALL PERFORMANCE\n")
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────────\n")
	total := summary.TotalMetrics
	fmt.Printf("Total Impressions:  %s\n", formatNumber(total.Impressions))
	fmt.Printf("Total Clicks:       %s\n", formatNumber(total.Clicks))
	fmt.Printf("Total Spend:        $%.2f\n", total.Spend)
	fmt.Printf("Overall CTR:        %.2f%%\n", total.CTR)
	fmt.Printf("Average CPM:        $%.2f\n", total.CPM)
	if total.CPC > 0 {
		fmt.Printf("Average CPC:        $%.2f\n", total.CPC)
	}
	fmt.Printf("\n")

	// Daily Breakdown
	if len(summary.DailyMetrics) > 0 {
		fmt.Printf("📅 DAILY BREAKDOWN\n")
		fmt.Printf("───────────────────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("Date        | Impressions | Clicks |   CTR   |   Spend   |   CPM   |   CPC   \n")
		fmt.Printf("------------|-------------|--------|---------|-----------|---------|----------\n")
		for _, dm := range summary.DailyMetrics {
			fmt.Printf("%-10s | %11s | %6s | %6.2f%% | $%8.2f | $%6.2f | $%6.2f\n",
				dm.Date.Format("2006-01-02"),
				formatNumber(dm.Impressions),
				formatNumber(dm.Clicks),
				dm.CTR,
				dm.Spend,
				dm.CPM,
				dm.CPC,
			)
		}
		fmt.Printf("\n")
	}

	// Line Item Breakdown
	if len(summary.LineItemMetrics) > 0 {
		fmt.Printf("📋 LINE ITEM BREAKDOWN\n")
		fmt.Printf("───────────────────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("Line Item ID | Impressions | Clicks |   CTR   |   Spend   |   CPM   |   CPC   \n")
		fmt.Printf("-------------|-------------|--------|---------|-----------|---------|----------\n")
		for _, li := range summary.LineItemMetrics {
			fmt.Printf("%12d | %11s | %6s | %6.2f%% | $%8.2f | $%6.2f | $%6.2f\n",
				li.LineItemID,
				formatNumber(li.Impressions),
				formatNumber(li.Clicks),
				li.CTR,
				li.Spend,
				li.CPM,
				li.CPC,
			)
		}
		fmt.Printf("\n")
	}

	// Top Creatives
	if len(summary.TopCreatives) > 0 {
		fmt.Printf("🎨 TOP PERFORMING CREATIVES\n")
		fmt.Printf("───────────────────────────────────────────────────────────────────────────────────\n")
		fmt.Printf("Creative ID | Impressions | Clicks |   CTR   |   Spend   \n")
		fmt.Printf("------------|-------------|--------|---------|----------\n")
		for _, c := range summary.TopCreatives {
			fmt.Printf("%11d | %11s | %6s | %6.2f%% | $%8.2f\n",
				c.CreativeID,
				formatNumber(c.Impressions),
				formatNumber(c.Clicks),
				c.CTR,
				c.Spend,
			)
		}
		fmt.Printf("\n")
	}

	// Insights
	fmt.Printf("💡 INSIGHTS & RECOMMENDATIONS\n")
	fmt.Printf("───────────────────────────────────────────────────────────────────────────────────\n")

	if total.CTR == 0 {
		fmt.Printf("⚠️  No clicks recorded - consider reviewing creative strategy\n")
	} else if total.CTR < 1.0 {
		fmt.Printf("⚠️  Low CTR (%.2f%%) - consider optimizing creatives or targeting\n", total.CTR)
	} else if total.CTR > 3.0 {
		fmt.Printf("✅ Excellent CTR (%.2f%%) - campaign performing well!\n", total.CTR)
	} else {
		fmt.Printf("✅ Good CTR (%.2f%%) - within normal range\n", total.CTR)
	}

	if len(summary.TopCreatives) > 1 {
		best := summary.TopCreatives[0]
		worst := summary.TopCreatives[len(summary.TopCreatives)-1]
		if best.CTR > worst.CTR*2 {
			fmt.Printf("📈 Creative %d is performing %.1fx better than Creative %d\n",
				best.CreativeID, best.CTR/worst.CTR, worst.CreativeID)
		}
	}

	// Line Item Performance Insights
	if len(summary.LineItemMetrics) > 1 {
		// Find best and worst performing line items by CTR
		bestLI := summary.LineItemMetrics[0]
		worstLI := summary.LineItemMetrics[0]
		for _, li := range summary.LineItemMetrics {
			if li.CTR > bestLI.CTR {
				bestLI = li
			}
			if li.CTR < worstLI.CTR && li.CTR > 0 {
				worstLI = li
			}
		}

		if bestLI.LineItemID != worstLI.LineItemID && bestLI.CTR > worstLI.CTR*2 {
			fmt.Printf("📊 Line Item %d has %.1fx better CTR (%.2f%%) than Line Item %d (%.2f%%)\n",
				bestLI.LineItemID, bestLI.CTR/worstLI.CTR, bestLI.CTR, worstLI.LineItemID, worstLI.CTR)
		}

		// Check for budget concentration
		totalSpend := total.Spend
		if totalSpend > 0 {
			for _, li := range summary.LineItemMetrics {
				spendShare := li.Spend / totalSpend * 100
				if spendShare > 50 {
					fmt.Printf("⚠️  Line Item %d is consuming %.1f%% of total campaign spend\n",
						li.LineItemID, spendShare)
					break
				}
			}
		}
	} else if len(summary.LineItemMetrics) == 0 {
		fmt.Printf("⚠️  No line item data available - check line item setup and tracking\n")
	}

	if total.Impressions > 0 && total.Clicks == 0 {
		fmt.Printf("🔍 Consider A/B testing different creative approaches\n")
	}

	fmt.Printf("═══════════════════════════════════════════════════════════════════════════════════\n")
}

// formatNumber formats large integers with comma separators for improved readability.
// Example: 1234567 becomes "1,234,567"
func formatNumber(n int64) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	// Add commas for thousands separators
	result := ""
	for i, digit := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(digit)
	}
	return result
}

// getEnv retrieves an environment variable value or returns a default value if not set.
// Used for configuration with fallback defaults.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
