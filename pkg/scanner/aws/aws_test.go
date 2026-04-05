package aws

import (
	"math"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestAWSScannerMetadata(t *testing.T) {
	scanner := NewScanner()
	if scanner.Name() != "AWS Resource Scanner" {
		t.Errorf("expected 'AWS Resource Scanner', got %q", scanner.Name())
	}
	if scanner.Provider() != "aws" {
		t.Errorf("expected 'aws', got %q", scanner.Provider())
	}
}

func TestCalculateP95(t *testing.T) {
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
			input:    []float64{10.0},
			expected: 10.0,
		},
		{
			name:     "sorted elements",
			input:    []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0},
			expected: 10.0,
		},
		{
			name:     "unsorted elements 20 points",
			input:    []float64{5.0, 1.0, 9.0, 3.0, 7.0, 2.0, 10.0, 4.0, 8.0, 6.0, 15.0, 12.0, 19.0, 13.0, 17.0, 11.0, 20.0, 14.0, 18.0, 16.0},
			expected: 19.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateP95(tt.input)
			if math.Abs(result-tt.expected) > 1e-6 {
				t.Errorf("calculateP95(%v) = %f, expected %f", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseTags(t *testing.T) {
	tags := []ec2types.Tag{
		{
			Key:   aws.String("Name"),
			Value: aws.String("web-server"),
		},
		{
			Key:   aws.String("Environment"),
			Value: aws.String("staging"),
		},
		{
			Key:   nil,
			Value: aws.String("invalid"),
		},
		{
			Key:   aws.String("incomplete"),
			Value: nil,
		},
	}

	parsed := parseTags(tags)

	if len(parsed) != 2 {
		t.Fatalf("expected 2 valid tags, got %d", len(parsed))
	}
	if parsed["Name"] != "web-server" {
		t.Errorf("expected Name tag 'web-server', got %q", parsed["Name"])
	}
	if parsed["Environment"] != "staging" {
		t.Errorf("expected Environment tag 'staging', got %q", parsed["Environment"])
	}
}
