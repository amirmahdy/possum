package pricing

import (
	"testing"
)

func TestGetEC2MonthlyCost(t *testing.T) {
	tests := []struct {
		instType string
		want     float64
	}{
		{"t3.micro", 7.60},
		{"m5.large", 70.00},
		{"c5.2xlarge", 248.00},
		{"custom.2xlarge", 240.00},
		{"custom.xlarge", 120.00},
		{"custom.large", 60.00},
		{"custom.medium", 30.00},
		{"custom.small", 15.00},
		{"unknown.type", DefaultFallbackInstance},
	}

	for _, tt := range tests {
		got := GetEC2MonthlyCost(tt.instType)
		if got != tt.want {
			t.Errorf("GetEC2MonthlyCost(%q) = %v, want %v", tt.instType, got, tt.want)
		}
	}
}

func TestGetGCPVMMonthlyCost(t *testing.T) {
	tests := []struct {
		machineType string
		want        float64
	}{
		{"e2-micro", 6.11},
		{"e2-medium", 24.46},
		{"n2-standard-4", 153.13},
		{"custom-micro", 6.00},
		{"custom-small", 12.00},
		{"custom-medium", 25.00},
		{"custom-standard-2", 50.00},
		{"custom-standard-4", 100.00},
		{"unknown-machine-type", DefaultFallbackVM},
	}

	for _, tt := range tests {
		got := GetGCPVMMonthlyCost(tt.machineType)
		if got != tt.want {
			t.Errorf("GetGCPVMMonthlyCost(%q) = %v, want %v", tt.machineType, got, tt.want)
		}
	}
}

func TestGetEBSMonthlyCost(t *testing.T) {
	tests := []struct {
		volType string
		sizeGB  float64
		want    float64
	}{
		{"gp2", 100, 10.00},
		{"gp3", 100, 8.00},
		{"io1", 100, 12.50},
		{"io2", 100, 12.50},
		{"st1", 100, 4.50},
		{"sc1", 100, 1.50},
		{"standard", 100, 5.00},
		{"unknown", 100, 8.00},
	}

	for _, tt := range tests {
		got := GetEBSMonthlyCost(tt.volType, tt.sizeGB)
		if got != tt.want {
			t.Errorf("GetEBSMonthlyCost(%q, %v) = %v, want %v", tt.volType, tt.sizeGB, got, tt.want)
		}
	}
}

func TestGetRDSMonthlyCost(t *testing.T) {
	tests := []struct {
		dbClass string
		want    float64
	}{
		{"db.t3.micro", 12.41},
		{"db.m5.large", 138.70},
		{"custom.2xlarge", 400.00},
		{"custom.micro", 12.50},
		{"unknown.class", DefaultFallbackRDS},
	}

	for _, tt := range tests {
		got := GetRDSMonthlyCost(tt.dbClass)
		if got != tt.want {
			t.Errorf("GetRDSMonthlyCost(%q) = %v, want %v", tt.dbClass, got, tt.want)
		}
	}
}

func TestGetCloudSQLMonthlyCost(t *testing.T) {
	tests := []struct {
		tier string
		want float64
	}{
		{"db-f1-micro", 7.67},
		{"db-n1-standard-1", 50.37},
		{"custom-micro", 7.67},
		{"unknown-tier", DefaultFallbackCloudSQL},
	}

	for _, tt := range tests {
		got := GetCloudSQLMonthlyCost(tt.tier)
		if got != tt.want {
			t.Errorf("GetCloudSQLMonthlyCost(%q) = %v, want %v", tt.tier, got, tt.want)
		}
	}
}
