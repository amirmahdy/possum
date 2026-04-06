package gcp

import (
	"context"
	"math"
	"testing"

	"possum/pkg/scanner"
)

func TestGCPScannerMetadata(t *testing.T) {
	scanner := NewScanner()
	if scanner.Name() != "GCP Resource Scanner" {
		t.Errorf("expected 'GCP Resource Scanner', got %q", scanner.Name())
	}
	if scanner.Provider() != "gcp" {
		t.Errorf("expected 'gcp', got %q", scanner.Provider())
	}
}

func TestGCPScanMissingProject(t *testing.T) {
	s := NewScanner()
	ctx := context.Background()
	_, err := s.Scan(ctx, scanner.ScanOptions{Project: ""})
	if err == nil {
		t.Error("expected error when project ID is missing, got nil")
	}
}

func TestCalculateP95GCP(t *testing.T) {
	tests := []struct {
		name     string
		input    []float64
		expected float64
	}{
		{
			name:     "empty slice",
			input:    []float64{},
			expected: 0.0,
		},
		{
			name:     "single element",
			input:    []float64{15.5},
			expected: 15.5,
		},
		{
			name:     "10 elements sorted",
			input:    []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0},
			expected: 10.0,
		},
		{
			name:     "20 elements unsorted",
			input:    []float64{5.0, 1.0, 9.0, 3.0, 7.0, 2.0, 10.0, 4.0, 8.0, 6.0, 15.0, 12.0, 19.0, 13.0, 17.0, 11.0, 20.0, 14.0, 18.0, 16.0},
			expected: 19.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateP95GCP(tt.input)
			if math.Abs(result-tt.expected) > 1e-6 {
				t.Errorf("calculateP95GCP(%v) = %f, expected %f", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetResourceNameFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{
			url:      "",
			expected: "",
		},
		{
			url:      "https://www.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a/machineTypes/e2-micro",
			expected: "e2-micro",
		},
		{
			url:      "projects/my-project/zones/us-central1-a",
			expected: "us-central1-a",
		},
		{
			url:      "pd-standard",
			expected: "pd-standard",
		},
	}

	for _, tt := range tests {
		result := getResourceNameFromURL(tt.url)
		if result != tt.expected {
			t.Errorf("getResourceNameFromURL(%q) = %q, expected %q", tt.url, result, tt.expected)
		}
	}
}
