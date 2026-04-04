package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"possum/pkg/scanner"
	"possum/pkg/scanner/aws"
	"possum/pkg/scanner/gcp"
)

func main() {
	providerFlag := flag.String("provider", "aws", "Cloud provider to scan: aws, gcp, or all")
	region := flag.String("region", "us-east-1", "AWS region to scan")
	profile := flag.String("profile", "", "AWS CLI profile name to authenticate with")
	project := flag.String("project", "", "GCP Project ID to scan (Required if scanning GCP)")
	credentials := flag.String("credentials", "", "Path to GCP Service Account JSON credentials file (optional)")
	cpuThreshold := flag.Float64("cpu-threshold", 2.0, "CPU % average threshold to define an idle resource")
	cpuP95Threshold := flag.Float64("cpu-p95-threshold", 5.0, "CPU % P95 threshold to define an idle resource")
	netThresholdMB := flag.Float64("network-mb-threshold", 500.0, "Network total MB transfer threshold over lookback window")
	diskOpsThreshold := flag.Float64("disk-ops-threshold", 1000.0, "Disk total IOPS threshold over lookback window")
	lookbackDays := flag.Int("lookback-days", 30, "Lookback window in days for idle evaluation")
	flag.Parse()

	prov := strings.ToLower(strings.TrimSpace(*providerFlag))

	opts := scanner.ScanOptions{
		Provider:         prov,
		Region:           *region,
		Profile:          *profile,
		Project:          *project,
		CredentialsFile:  *credentials,
		CPUAvgThreshold:  *cpuThreshold,
		CPUP95Threshold:  *cpuP95Threshold,
		NetThresholdMB:   *netThresholdMB,
		DiskOpsThreshold: *diskOpsThreshold,
		LookbackDays:     *lookbackDays,
	}

	var activeScanners []scanner.Scanner

	switch prov {
	case "aws":
		activeScanners = append(activeScanners, aws.NewScanner())
	case "gcp":
		if *project == "" {
			fmt.Fprintln(os.Stderr, "Error: -project flag is required when scanning GCP")
			flag.Usage()
			os.Exit(1)
		}
		activeScanners = append(activeScanners, gcp.NewScanner())
	case "all", "both":
		activeScanners = append(activeScanners, aws.NewScanner())
		if *project != "" {
			activeScanners = append(activeScanners, gcp.NewScanner())
		} else {
			fmt.Println("Notice: GCP project not provided (-project), skipping GCP scan in 'all' mode.")
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: unsupported provider %q. Supported providers: aws, gcp, all\n", prov)
		flag.Usage()
		os.Exit(1)
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Starting Possum Cloud Waste Scan (Provider: %s)\n", strings.ToUpper(prov))
	fmt.Println(strings.Repeat("=", 60))

	ctx := context.Background()
	var allFindings []scanner.Finding

	for _, s := range activeScanners {
		fmt.Printf("\nExecuting %s...\n", s.Name())
		findings, err := s.Scan(ctx, opts)
		if err != nil {
			fmt.Printf("Error running %s: %v\n", s.Name(), err)
			continue
		}
		allFindings = append(allFindings, findings...)
	}

	var totalSavings float64
	for _, f := range allFindings {
		totalSavings += f.EstimatedMonthlySavings
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("POSSUM CLOUD WASTE ANALYSIS REPORT")
	fmt.Println(strings.Repeat("=", 60))

	if len(allFindings) == 0 {
		fmt.Println("Excellent! No idle or unassociated resources found.")
	} else {
		for idx, finding := range allFindings {
			fmt.Printf("\n[%d] %s: %s\n", idx+1, finding.Type, finding.ID)
			fmt.Printf("    Details:   %s\n", finding.Details)
			if len(finding.Tags) > 0 {
				fmt.Printf("    Tags:      %v\n", finding.Tags)
			} else {
				fmt.Println("    Tags:      None")
			}
			fmt.Printf("    Est. Cost: $%.2f/month\n", finding.EstimatedMonthlySavings)
		}

		fmt.Println("\n" + strings.Repeat("-", 60))
		fmt.Printf("TOTAL POTENTIAL SAVINGS: $%.2f / month\n", totalSavings)
		fmt.Println(strings.Repeat("-", 60))
	}
}
