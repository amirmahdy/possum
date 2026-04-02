package aws

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"possum/pkg/pricing"
	"possum/pkg/scanner"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// AWSScanner implements the scanner.Scanner interface for AWS resources.
type AWSScanner struct{}

// NewScanner returns a new instance of AWSScanner.
func NewScanner() scanner.Scanner {
	return &AWSScanner{}
}

func (s *AWSScanner) Name() string {
	return "AWS Resource Scanner"
}

func (s *AWSScanner) Provider() string {
	return "aws"
}

func (s *AWSScanner) Scan(ctx context.Context, opts scanner.ScanOptions) ([]scanner.Finding, error) {
	region := opts.Region
	if region == "" {
		region = "us-east-1"
	}

	var cfg aws.Config
	var err error
	if opts.Profile != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithSharedConfigProfile(opts.Profile),
			config.WithRegion(region),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	ec2Client := ec2.NewFromConfig(cfg)
	cwClient := cloudwatch.NewFromConfig(cfg)

	fmt.Println("\nScanning AWS: Unattached EBS Volumes...")
	ebsFindings, err := scanUnattachedEBS(ctx, ec2Client)
	if err != nil {
		fmt.Printf("Error scanning EBS volumes: %v\n", err)
	} else {
		fmt.Printf("Found %d unattached EBS volumes.\n", len(ebsFindings))
	}

	fmt.Println("\nScanning AWS: Unassociated Elastic IPs...")
	eipFindings, err := scanUnassociatedEIPs(ctx, ec2Client)
	if err != nil {
		fmt.Printf("Error scanning Elastic IPs: %v\n", err)
	} else {
		fmt.Printf("Found %d unassociated Elastic IPs.\n", len(eipFindings))
	}

	fmt.Printf("\nScanning AWS: Idle EC2 Instances (Avg CPU < %.2f%%, P95 CPU < %.2f%%, Net < %.2fMB, Disk Ops < %.0f over %d days)...\n",
		opts.CPUAvgThreshold, opts.CPUP95Threshold, opts.NetThresholdMB, opts.DiskOpsThreshold, opts.LookbackDays)
	ec2Findings, err := scanIdleEC2(ctx, ec2Client, cwClient, opts.CPUAvgThreshold, opts.CPUP95Threshold, opts.NetThresholdMB, opts.DiskOpsThreshold, opts.LookbackDays)
	if err != nil {
		fmt.Printf("Error scanning EC2 instances: %v\n", err)
	} else {
		fmt.Printf("Found %d idle EC2 instances.\n", len(ec2Findings))
	}

	var findings []scanner.Finding
	findings = append(findings, ebsFindings...)
	findings = append(findings, eipFindings...)
	findings = append(findings, ec2Findings...)

	return findings, nil
}

func parseTags(tags []ec2types.Tag) map[string]string {
	tagMap := make(map[string]string)
	for _, t := range tags {
		if t.Key != nil && t.Value != nil {
			tagMap[*t.Key] = *t.Value
		}
	}
	return tagMap
}

func scanUnattachedEBS(ctx context.Context, client *ec2.Client) ([]scanner.Finding, error) {
	var findings []scanner.Finding

	input := &ec2.DescribeVolumesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("status"),
				Values: []string{"available"},
			},
		},
	}

	paginator := ec2.NewDescribeVolumesPaginator(client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, vol := range page.Volumes {
			volID := aws.ToString(vol.VolumeId)
			sizeGB := aws.ToInt32(vol.Size)
			volType := string(vol.VolumeType)
			createTime := ""
			if vol.CreateTime != nil {
				createTime = vol.CreateTime.Format("2006-01-02 15:04:05")
			}
			tags := parseTags(vol.Tags)

			savings := pricing.GetEBSMonthlyCost(volType, float64(sizeGB))

			findings = append(findings, scanner.Finding{
				ID:                      volID,
				Type:                    fmt.Sprintf("EBS Volume (%s)", volType),
				Details:                 fmt.Sprintf("Size: %d GB, Created: %s", sizeGB, createTime),
				Tags:                    tags,
				EstimatedMonthlySavings: savings,
			})
		}
	}

	return findings, nil
}

func scanUnassociatedEIPs(ctx context.Context, client *ec2.Client) ([]scanner.Finding, error) {
	var findings []scanner.Finding

	output, err := client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
	if err != nil {
		return nil, err
	}

	for _, addr := range output.Addresses {
		if addr.AssociationId == nil || *addr.AssociationId == "" {
			publicIP := aws.ToString(addr.PublicIp)
			allocID := aws.ToString(addr.AllocationId)
			tags := parseTags(addr.Tags)

			findings = append(findings, scanner.Finding{
				ID:                      allocID,
				Type:                    "Elastic IP",
				Details:                 fmt.Sprintf("Public IP: %s", publicIP),
				Tags:                    tags,
				EstimatedMonthlySavings: pricing.EIPMonthlyCost,
			})
		}
	}

	return findings, nil
}

type EC2MetricStats struct {
	AvgCPU       float64
	P95CPU       float64
	TotalNetMB   float64
	TotalDiskOps float64
	HasData      bool
}

func calculateP95(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	idx := int(math.Ceil(0.95*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func getEC2MetricSum(ctx context.Context, client *cloudwatch.Client, instanceID string, metricName string, lookbackDays int) (float64, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -lookbackDays)

	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/EC2"),
		MetricName: aws.String(metricName),
		Dimensions: []cwtypes.Dimension{
			{
				Name:  aws.String("InstanceId"),
				Value: aws.String(instanceID),
			},
		},
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(86400),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticSum},
	}

	output, err := client.GetMetricStatistics(ctx, input)
	if err != nil {
		return 0, err
	}

	var total float64
	for _, dp := range output.Datapoints {
		if dp.Sum != nil {
			total += *dp.Sum
		}
	}
	return total, nil
}

func getEC2MultiMetrics(ctx context.Context, client *cloudwatch.Client, instanceID string, lookbackDays int) (EC2MetricStats, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -lookbackDays)

	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/EC2"),
		MetricName: aws.String("CPUUtilization"),
		Dimensions: []cwtypes.Dimension{
			{
				Name:  aws.String("InstanceId"),
				Value: aws.String(instanceID),
			},
		},
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(3600),
		Statistics: []cwtypes.Statistic{cwtypes.StatisticAverage},
	}

	output, err := client.GetMetricStatistics(ctx, input)
	if err != nil {
		return EC2MetricStats{}, err
	}

	if len(output.Datapoints) == 0 {
		return EC2MetricStats{HasData: false}, nil
	}

	var cpuValues []float64
	var sumCPU float64

	for _, dp := range output.Datapoints {
		if dp.Average != nil {
			val := *dp.Average
			cpuValues = append(cpuValues, val)
			sumCPU += val
		}
	}

	if len(cpuValues) == 0 {
		return EC2MetricStats{HasData: false}, nil
	}

	avgCPU := sumCPU / float64(len(cpuValues))
	p95CPU := calculateP95(cpuValues)

	netInBytes, _ := getEC2MetricSum(ctx, client, instanceID, "NetworkIn", lookbackDays)
	netOutBytes, _ := getEC2MetricSum(ctx, client, instanceID, "NetworkOut", lookbackDays)
	totalNetMB := (netInBytes + netOutBytes) / (1024 * 1024)

	diskReadOps, _ := getEC2MetricSum(ctx, client, instanceID, "DiskReadOps", lookbackDays)
	diskWriteOps, _ := getEC2MetricSum(ctx, client, instanceID, "DiskWriteOps", lookbackDays)
	totalDiskOps := diskReadOps + diskWriteOps

	return EC2MetricStats{
		AvgCPU:       avgCPU,
		P95CPU:       p95CPU,
		TotalNetMB:   totalNetMB,
		TotalDiskOps: totalDiskOps,
		HasData:      true,
	}, nil
}

func scanIdleEC2(ctx context.Context, ec2Client *ec2.Client, cwClient *cloudwatch.Client, cpuAvgThreshold, cpuP95Threshold, netThresholdMB, diskOpsThreshold float64, lookbackDays int) ([]scanner.Finding, error) {
	var findings []scanner.Finding

	input := &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"running"},
			},
		},
	}

	paginator := ec2.NewDescribeInstancesPaginator(ec2Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, reservation := range page.Reservations {
			for _, inst := range reservation.Instances {
				instID := aws.ToString(inst.InstanceId)
				instType := string(inst.InstanceType)
				launchTime := ""
				if inst.LaunchTime != nil {
					launchTime = inst.LaunchTime.Format("2006-01-02 15:04:05")
				}
				tags := parseTags(inst.Tags)
				name := tags["Name"]
				if name == "" {
					name = "Unnamed"
				}

				metrics, err := getEC2MultiMetrics(ctx, cwClient, instID, lookbackDays)
				if err != nil {
					log.Printf("Warning: failed to query CloudWatch metrics for %s: %v\n", instID, err)
					continue
				}

				if metrics.HasData && metrics.AvgCPU < cpuAvgThreshold && metrics.P95CPU < cpuP95Threshold && metrics.TotalNetMB < netThresholdMB && metrics.TotalDiskOps < diskOpsThreshold {
					cost := pricing.GetEC2MonthlyCost(instType)

					findings = append(findings, scanner.Finding{
						ID:   instID,
						Type: fmt.Sprintf("Idle EC2 Instance (%s)", instType),
						Details: fmt.Sprintf(
							"Name: %s, Avg CPU: %.2f%%, P95 CPU: %.2f%%, Net Transfer: %.2f MB, Disk IOPS: %.0f over %d days, Launched: %s",
							name, metrics.AvgCPU, metrics.P95CPU, metrics.TotalNetMB, metrics.TotalDiskOps, lookbackDays, launchTime,
						),
						Tags:                    tags,
						EstimatedMonthlySavings: math.Round(cost*100) / 100,
					})
				}
			}
		}
	}

	return findings, nil
}
