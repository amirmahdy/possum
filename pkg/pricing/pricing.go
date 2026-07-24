package pricing

import (
	"strings"
)

// AWS Pricing Constants (based on standard us-east-1 rates)
const (
	EIPMonthlyCost           = 3.60  // ~$0.005/hour for unassociated Elastic IP
	EBSGP2GBMonthlyCost      = 0.10  // $0.10 per GB-month
	EBSGP3GBMonthlyCost      = 0.08  // $0.08 per GB-month
	EBSIO1GBMonthlyCost      = 0.125 // $0.125 per GB-month
	EBSIO2GBMonthlyCost      = 0.125 // $0.125 per GB-month
	EBSST1GBMonthlyCost      = 0.045 // $0.045 per GB-month
	EBSSC1GBMonthlyCost      = 0.015 // $0.015 per GB-month
	EBSStandardGBMonthlyCost = 0.05  // $0.05 per GB-month
	DefaultFallbackInstance  = 15.0  // Fallback monthly cost for unknown EC2 instance type
	LoadBalancerMonthlyCost  = 18.00 // ~$0.0225/hour base cost for ALB/NLB/CLB
	DefaultFallbackRDS       = 35.0  // Fallback monthly cost for unknown RDS instance
)

// GetEBSMonthlyCost calculates estimated monthly cost for an EBS volume based on volume type and size in GB.
func GetEBSMonthlyCost(volType string, sizeGB float64) float64 {
	var rate float64
	switch strings.ToLower(volType) {
	case "gp2":
		rate = EBSGP2GBMonthlyCost
	case "gp3":
		rate = EBSGP3GBMonthlyCost
	case "io1":
		rate = EBSIO1GBMonthlyCost
	case "io2":
		rate = EBSIO2GBMonthlyCost
	case "st1":
		rate = EBSST1GBMonthlyCost
	case "sc1":
		rate = EBSSC1GBMonthlyCost
	case "standard":
		rate = EBSStandardGBMonthlyCost
	default:
		rate = EBSGP3GBMonthlyCost
	}
	return sizeGB * rate
}


// GCP Pricing Constants (based on standard us-central1 rates)
const (
	StaticIPMonthlyCost     = 2.92 // ~$0.004/hour for unassociated static IP
	PDStandardGBMonthlyCost = 0.04 // $0.04 per GB-month for standard persistent disk
	PDBalancedGBMonthlyCost = 0.10 // $0.10 per GB-month for balanced persistent disk
	DefaultFallbackVM       = 25.0 // Fallback monthly cost for unknown machine type
	DefaultFallbackCloudSQL = 30.0 // Fallback monthly cost for unknown Cloud SQL instance
)

// Instance monthly cost lookup table for common EC2 types
var ec2MonthlyCosts = map[string]float64{
	"t3.nano": 3.80, "t3.micro": 7.60, "t3.small": 15.20, "t3.medium": 30.40, "t3.large": 60.80, "t3.xlarge": 121.60, "t3.2xlarge": 243.20,
	"t2.nano": 4.20, "t2.micro": 8.50, "t2.small": 17.00, "t2.medium": 34.00, "t2.large": 68.00,
	"m5.large": 70.00, "m5.xlarge": 140.00, "m5.2xlarge": 280.00,
	"c5.large": 62.00, "c5.xlarge": 124.00, "c5.2xlarge": 248.00,
	"r5.large": 92.00, "r5.xlarge": 184.00, "r5.2xlarge": 368.00,
}

// RDS monthly cost lookup table for common DB instance classes
var rdsMonthlyCosts = map[string]float64{
	"db.t3.micro": 12.41, "db.t3.small": 24.82, "db.t3.medium": 49.64, "db.t3.large": 99.28, "db.t3.xlarge": 198.56, "db.t3.2xlarge": 397.12,
	"db.t2.micro": 12.41, "db.t2.small": 24.82, "db.t2.medium": 49.64, "db.t2.large": 99.28,
	"db.m5.large": 138.70, "db.m5.xlarge": 277.40, "db.m5.2xlarge": 554.80,
	"db.c5.large": 122.00, "db.c5.xlarge": 244.00, "db.c5.2xlarge": 488.00,
	"db.r5.large": 175.20, "db.r5.xlarge": 350.40, "db.r5.2xlarge": 700.80,
}

// Machine type monthly cost lookup table for common GCP types
var gcpMachineCosts = map[string]float64{
	"e2-micro": 6.11, "e2-small": 12.23, "e2-medium": 24.46,
	"e2-standard-2": 48.92, "e2-standard-4": 97.85,
	"n2-standard-2": 76.57, "n2-standard-4": 153.13,
	"n1-standard-1": 24.27, "n1-standard-2": 48.54, "n1-standard-4": 97.08,
}

// Cloud SQL tier monthly cost lookup table
var cloudSQLMonthlyCosts = map[string]float64{
	"db-f1-micro": 7.67, "db-g1-small": 25.55,
	"db-n1-standard-1": 50.37, "db-n1-standard-2": 100.74, "db-n1-standard-4": 201.48, "db-n1-standard-8": 402.96,
	"db-custom-1-3840": 45.00, "db-custom-2-7680": 90.00,
}

// GetEC2MonthlyCost resolves the estimated monthly cost for an AWS EC2 instance type.
// It checks the exact lookup table first, falls back to size-tier heuristics, and finally returns DefaultFallbackInstance.
func GetEC2MonthlyCost(instType string) float64 {
	if cost, found := ec2MonthlyCosts[instType]; found {
		return cost
	}
	switch {
	case strings.Contains(instType, "2xlarge"):
		return 240.0
	case strings.Contains(instType, "xlarge"):
		return 120.0
	case strings.Contains(instType, "large"):
		return 60.0
	case strings.Contains(instType, "medium"):
		return 30.0
	case strings.Contains(instType, "small"):
		return 15.0
	default:
		return DefaultFallbackInstance
	}
}

// GetRDSMonthlyCost resolves the estimated monthly cost for an AWS RDS DB instance class.
func GetRDSMonthlyCost(dbClass string) float64 {
	if cost, found := rdsMonthlyCosts[strings.ToLower(dbClass)]; found {
		return cost
	}
	switch {
	case strings.Contains(dbClass, "2xlarge"):
		return 400.0
	case strings.Contains(dbClass, "xlarge"):
		return 200.0
	case strings.Contains(dbClass, "large"):
		return 100.0
	case strings.Contains(dbClass, "medium"):
		return 50.0
	case strings.Contains(dbClass, "small"):
		return 25.0
	case strings.Contains(dbClass, "micro"):
		return 12.50
	default:
		return DefaultFallbackRDS
	}
}

// GetGCPVMMonthlyCost resolves the estimated monthly cost for a GCP Compute Engine machine type.
// It checks the exact lookup table first, falls back to size-tier heuristics, and finally returns DefaultFallbackVM.
func GetGCPVMMonthlyCost(machineType string) float64 {
	if cost, found := gcpMachineCosts[machineType]; found {
		return cost
	}
	switch {
	case strings.Contains(machineType, "micro"):
		return 6.0
	case strings.Contains(machineType, "small"):
		return 12.0
	case strings.Contains(machineType, "medium"):
		return 25.0
	case strings.Contains(machineType, "standard-2"):
		return 50.0
	case strings.Contains(machineType, "standard-4"):
		return 100.0
	default:
		return DefaultFallbackVM
	}
}

// GetCloudSQLMonthlyCost resolves the estimated monthly cost for a GCP Cloud SQL tier.
func GetCloudSQLMonthlyCost(tier string) float64 {
	if cost, found := cloudSQLMonthlyCosts[strings.ToLower(tier)]; found {
		return cost
	}
	switch {
	case strings.Contains(tier, "micro"):
		return 7.67
	case strings.Contains(tier, "small"):
		return 25.55
	case strings.Contains(tier, "standard-1"):
		return 50.0
	case strings.Contains(tier, "standard-2"):
		return 100.0
	case strings.Contains(tier, "standard-4"):
		return 200.0
	default:
		return DefaultFallbackCloudSQL
	}
}
