package scanner

import "context"

// Finding represents an identified idle or unused cloud resource.
type Finding struct {
	ID                      string            `json:"id"`
	Type                    string            `json:"type"`
	Details                 string            `json:"details"`
	Tags                    map[string]string `json:"tags"`
	EstimatedMonthlySavings float64           `json:"estimated_monthly_savings"`
}

// ScanOptions contains common options and parameters for running resource scans.
type ScanOptions struct {
	Provider        string  // "aws", "gcp", or "all"
	Region          string  // AWS region to scan
	Profile         string  // AWS CLI profile name
	Project         string  // GCP project ID
	CredentialsFile string  // GCP service account credentials file path
	CPUAvgThreshold float64 // Average CPU % threshold to define idle resource
	CPUP95Threshold float64 // P95 CPU % threshold to define idle resource
	NetThresholdMB   float64 // Network total MB transfer threshold
	DiskOpsThreshold float64 // Disk total IOPS threshold over lookback window
	LookbackDays     int     // Lookback window in days
}

// Scanner is the common interface implemented by cloud provider resource scanners.
type Scanner interface {
	Name() string
	Provider() string
	Scan(ctx context.Context, opts ScanOptions) ([]Finding, error)
}
