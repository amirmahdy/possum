package gcp

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"possum/pkg/pricing"
	"possum/pkg/scanner"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	monitoringpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GCPScanner implements the scanner.Scanner interface for GCP resources.
type GCPScanner struct{}

// NewScanner returns a new instance of GCPScanner.
func NewScanner() scanner.Scanner {
	return &GCPScanner{}
}

func (s *GCPScanner) Name() string {
	return "GCP Resource Scanner"
}

func (s *GCPScanner) Provider() string {
	return "gcp"
}

func (s *GCPScanner) Scan(ctx context.Context, opts scanner.ScanOptions) ([]scanner.Finding, error) {
	if opts.Project == "" {
		return nil, fmt.Errorf("GCP project ID (-project) is required for GCP scan")
	}

	var clientOpts []option.ClientOption
	if opts.CredentialsFile != "" {
		clientOpts = append(clientOpts, option.WithCredentialsFile(opts.CredentialsFile))
	}

	fmt.Println("\nScanning GCP: Unattached Persistent Disks...")
	diskFindings, err := scanUnattachedDisks(ctx, opts.Project, clientOpts)
	if err != nil {
		fmt.Printf("Error scanning Persistent Disks: %v\n", err)
	} else {
		fmt.Printf("Found %d unattached Persistent Disks.\n", len(diskFindings))
	}

	fmt.Println("\nScanning GCP: Unassociated Static IPs...")
	ipFindings, err := scanUnassociatedIPs(ctx, opts.Project, clientOpts)
	if err != nil {
		fmt.Printf("Error scanning Static IPs: %v\n", err)
	} else {
		fmt.Printf("Found %d unassociated Static IPs.\n", len(ipFindings))
	}

	fmt.Printf("\nScanning GCP: Idle VMs (Avg CPU < %.2f%%, P95 CPU < %.2f%%, Net < %.2fMB over %d days)...\n",
		opts.CPUAvgThreshold, opts.CPUP95Threshold, opts.NetThresholdMB, opts.LookbackDays)
	vmFindings, err := scanIdleVMs(ctx, opts.Project, clientOpts, opts.CPUAvgThreshold, opts.CPUP95Threshold, opts.NetThresholdMB, opts.LookbackDays)
	if err != nil {
		fmt.Printf("Error scanning VM instances: %v\n", err)
	} else {
		fmt.Printf("Found %d idle VM instances.\n", len(vmFindings))
	}

	fmt.Println("\nScanning GCP: Idle Cloud SQL Instances...")
	sqlFindings, err := scanIdleCloudSQL(ctx, opts.Project, clientOpts, opts.CPUAvgThreshold, opts.LookbackDays)
	if err != nil {
		fmt.Printf("Error scanning Cloud SQL instances: %v\n", err)
	} else {
		fmt.Printf("Found %d idle Cloud SQL instances.\n", len(sqlFindings))
	}

	var findings []scanner.Finding
	findings = append(findings, diskFindings...)
	findings = append(findings, ipFindings...)
	findings = append(findings, vmFindings...)
	findings = append(findings, sqlFindings...)

	return findings, nil
}

func scanUnattachedDisks(ctx context.Context, projectID string, opts []option.ClientOption) ([]scanner.Finding, error) {
	var findings []scanner.Finding

	disksClient, err := compute.NewDisksRESTClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create disks client: %w", err)
	}
	defer disksClient.Close()

	req := &computepb.AggregatedListDisksRequest{
		Project: projectID,
	}

	it := disksClient.AggregatedList(ctx, req)
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		for _, disk := range resp.Value.GetDisks() {
			if disk == nil {
				continue
			}
			if len(disk.GetUsers()) == 0 && disk.GetStatus() == "READY" {
				diskName := disk.GetName()
				diskTypeStr := disk.GetType()
				sizeGB := disk.GetSizeGb()
				zone := getResourceNameFromURL(disk.GetZone())

				rate := pricing.PDStandardGBMonthlyCost
				if strings.Contains(diskTypeStr, "pd-balanced") {
					rate = pricing.PDBalancedGBMonthlyCost
				} else if strings.Contains(diskTypeStr, "pd-ssd") {
					rate = 0.17
				}

				savings := float64(sizeGB) * rate

				findings = append(findings, scanner.Finding{
					ID:   diskName,
					Type: fmt.Sprintf("Persistent Disk (%s)", getResourceNameFromURL(diskTypeStr)),
					Details: fmt.Sprintf(
						"Zone: %s, Size: %d GB, Status: READY (Unattached)",
						zone, sizeGB,
					),
					Tags:                    disk.GetLabels(),
					EstimatedMonthlySavings: savings,
				})
			}
		}
	}

	return findings, nil
}

func scanUnassociatedIPs(ctx context.Context, projectID string, opts []option.ClientOption) ([]scanner.Finding, error) {
	var findings []scanner.Finding

	addressesClient, err := compute.NewAddressesRESTClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create addresses client: %w", err)
	}
	defer addressesClient.Close()

	req := &computepb.AggregatedListAddressesRequest{
		Project: projectID,
	}

	it := addressesClient.AggregatedList(ctx, req)
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		for _, addr := range resp.Value.GetAddresses() {
			if addr == nil {
				continue
			}
			if addr.GetStatus() == "RESERVED" {
				ipName := addr.GetName()
				address := addr.GetAddress()
				region := getResourceNameFromURL(addr.GetRegion())
				if region == "" {
					region = "global"
				}

				findings = append(findings, scanner.Finding{
					ID:                      ipName,
					Type:                    "Static IP Address",
					Details:                 fmt.Sprintf("Region: %s, IP Address: %s", region, address),
					Tags:                    addr.GetLabels(),
					EstimatedMonthlySavings: pricing.StaticIPMonthlyCost,
				})
			}
		}
	}

	return findings, nil
}

func calculateP95GCP(values []float64) float64 {
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

func scanIdleVMs(ctx context.Context, projectID string, opts []option.ClientOption, cpuAvgThreshold, cpuP95Threshold, netThresholdMB float64, lookbackDays int) ([]scanner.Finding, error) {
	var findings []scanner.Finding

	instancesClient, err := compute.NewInstancesRESTClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create instances client: %w", err)
	}
	defer instancesClient.Close()

	instanceMap := make(map[string]*computepb.Instance)
	listReq := &computepb.AggregatedListInstancesRequest{
		Project: projectID,
	}

	it := instancesClient.AggregatedList(ctx, listReq)
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		for _, inst := range resp.Value.GetInstances() {
			if inst == nil {
				continue
			}
			if inst.GetStatus() == "RUNNING" {
				idStr := fmt.Sprintf("%d", inst.GetId())
				instanceMap[idStr] = inst
			}
		}
	}

	metricClient, err := monitoring.NewMetricClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create monitoring client: %w", err)
	}
	defer metricClient.Close()

	now := time.Now()
	startTime := now.AddDate(0, 0, -lookbackDays)

	sentBytesMap := getGCPNetworkBytes(ctx, metricClient, projectID, "compute.googleapis.com/instance/network/sent_bytes_count", startTime, now)
	recvBytesMap := getGCPNetworkBytes(ctx, metricClient, projectID, "compute.googleapis.com/instance/network/received_bytes_count", startTime, now)

	metricFilter := `metric.type="compute.googleapis.com/instance/cpu/utilization" AND resource.type="gce_instance"`

	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", projectID),
		Filter: metricFilter,
		Interval: &monitoringpb.TimeInterval{
			StartTime: timestamppb.New(startTime),
			EndTime:   timestamppb.New(now),
		},
		View: monitoringpb.ListTimeSeriesRequest_FULL,
	}

	tsIter := metricClient.ListTimeSeries(ctx, req)
	for {
		ts, err := tsIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			fmt.Printf("Warning: failed to query Cloud Monitoring metric time series: %v\n", err)
			break
		}

		instanceID := ts.GetResource().GetLabels()["instance_id"]
		inst, ok := instanceMap[instanceID]
		if !ok {
			continue
		}

		var cpuValues []float64
		var sum float64
		for _, point := range ts.GetPoints() {
			val := point.GetValue().GetDoubleValue() * 100.0
			cpuValues = append(cpuValues, val)
			sum += val
		}

		if len(cpuValues) > 0 {
			avgCPU := sum / float64(len(cpuValues))
			p95CPU := calculateP95GCP(cpuValues)
			totalNetBytes := sentBytesMap[instanceID] + recvBytesMap[instanceID]
			estimatedNetMB := totalNetBytes / (1024.0 * 1024.0)

			if avgCPU < cpuAvgThreshold && p95CPU < cpuP95Threshold && estimatedNetMB < netThresholdMB {
				machineTypeFull := inst.GetMachineType()
				machineType := getResourceNameFromURL(machineTypeFull)
				zone := getResourceNameFromURL(inst.GetZone())

				cost := pricing.GetGCPVMMonthlyCost(machineType)

				findings = append(findings, scanner.Finding{
					ID:   inst.GetName(),
					Type: fmt.Sprintf("Idle VM (%s)", machineType),
					Details: fmt.Sprintf(
						"Zone: %s, Avg CPU: %.2f%%, P95 CPU: %.2f%%, Net Transfer: %.2f MB over %d days, Status: RUNNING",
						zone, avgCPU, p95CPU, estimatedNetMB, lookbackDays,
					),
					Tags:                    inst.GetLabels(),
					EstimatedMonthlySavings: math.Round(cost*100) / 100,
				})
			}
		}
	}

	return findings, nil
}

func getGCPNetworkBytes(ctx context.Context, metricClient *monitoring.MetricClient, projectID string, metricType string, startTime, endTime time.Time) map[string]float64 {
	bytesMap := make(map[string]float64)
	metricFilter := fmt.Sprintf(`metric.type="%s" AND resource.type="gce_instance"`, metricType)

	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", projectID),
		Filter: metricFilter,
		Interval: &monitoringpb.TimeInterval{
			StartTime: timestamppb.New(startTime),
			EndTime:   timestamppb.New(endTime),
		},
		Aggregation: &monitoringpb.Aggregation{
			AlignmentPeriod:  durationpb.New(time.Hour),
			PerSeriesAligner: monitoringpb.Aggregation_ALIGN_DELTA,
		},
		View: monitoringpb.ListTimeSeriesRequest_FULL,
	}

	tsIter := metricClient.ListTimeSeries(ctx, req)
	for {
		ts, err := tsIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			fmt.Printf("Warning: failed to query Cloud Monitoring metric time series (%s): %v\n", metricType, err)
			break
		}

		instanceID := ts.GetResource().GetLabels()["instance_id"]
		var total float64
		for _, point := range ts.GetPoints() {
			val := point.GetValue()
			if val.GetInt64Value() != 0 {
				total += float64(val.GetInt64Value())
			} else {
				total += val.GetDoubleValue()
			}
		}
		bytesMap[instanceID] += total
	}

	return bytesMap
}

func getResourceNameFromURL(url string) string {
	if url == "" {
		return ""
	}
	parts := strings.Split(url, "/")
	return parts[len(parts)-1]
}

func scanIdleCloudSQL(ctx context.Context, projectID string, opts []option.ClientOption, cpuAvgThreshold float64, lookbackDays int) ([]scanner.Finding, error) {
	var findings []scanner.Finding

	sqlService, err := sqladmin.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLAdmin client: %w", err)
	}

	instancesList, err := sqlService.Instances.List(projectID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list Cloud SQL instances: %w", err)
	}

	metricClient, err := monitoring.NewMetricClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create monitoring client: %w", err)
	}
	defer metricClient.Close()

	now := time.Now()
	startTime := now.AddDate(0, 0, -lookbackDays)

	for _, inst := range instancesList.Items {
		if inst.State != "RUNNABLE" {
			continue
		}

		instName := inst.Name
		tier := ""
		if inst.Settings != nil {
			tier = inst.Settings.Tier
		}
		dbVersion := inst.DatabaseVersion

		connCount := getCloudSQLMetricSum(ctx, metricClient, projectID, instName, "cloudsql.googleapis.com/database/network/connections", startTime, now)
		avgCPU := getCloudSQLMetricAverage(ctx, metricClient, projectID, instName, "cloudsql.googleapis.com/database/cpu/utilization", startTime, now)

		if connCount == 0 && avgCPU < cpuAvgThreshold {
			cost := pricing.GetCloudSQLMonthlyCost(tier)

			var tagMap map[string]string
			if inst.Settings != nil && len(inst.Settings.UserLabels) > 0 {
				tagMap = inst.Settings.UserLabels
			}

			findings = append(findings, scanner.Finding{
				ID:   instName,
				Type: fmt.Sprintf("Idle Cloud SQL Instance (%s)", tier),
				Details: fmt.Sprintf(
					"Database Version: %s, Tier: %s, Connections: %.0f, Avg CPU: %.2f%% over %d days",
					dbVersion, tier, connCount, avgCPU, lookbackDays,
				),
				Tags:                    tagMap,
				EstimatedMonthlySavings: math.Round(cost*100) / 100,
			})
		}
	}

	return findings, nil
}

func getCloudSQLMetricSum(ctx context.Context, metricClient *monitoring.MetricClient, projectID string, instanceName string, metricType string, startTime, endTime time.Time) float64 {
	metricFilter := fmt.Sprintf(`metric.type="%s" AND resource.type="cloudsql_database" AND resource.label.database_id="%s:%s"`, metricType, projectID, instanceName)

	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", projectID),
		Filter: metricFilter,
		Interval: &monitoringpb.TimeInterval{
			StartTime: timestamppb.New(startTime),
			EndTime:   timestamppb.New(endTime),
		},
		View: monitoringpb.ListTimeSeriesRequest_FULL,
	}

	tsIter := metricClient.ListTimeSeries(ctx, req)
	var total float64
	for {
		ts, err := tsIter.Next()
		if err == iterator.Done || err != nil {
			break
		}
		for _, point := range ts.GetPoints() {
			val := point.GetValue()
			if val.GetInt64Value() != 0 {
				total += float64(val.GetInt64Value())
			} else {
				total += val.GetDoubleValue()
			}
		}
	}
	return total
}

func getCloudSQLMetricAverage(ctx context.Context, metricClient *monitoring.MetricClient, projectID string, instanceName string, metricType string, startTime, endTime time.Time) float64 {
	metricFilter := fmt.Sprintf(`metric.type="%s" AND resource.type="cloudsql_database" AND resource.label.database_id="%s:%s"`, metricType, projectID, instanceName)

	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   fmt.Sprintf("projects/%s", projectID),
		Filter: metricFilter,
		Interval: &monitoringpb.TimeInterval{
			StartTime: timestamppb.New(startTime),
			EndTime:   timestamppb.New(endTime),
		},
		View: monitoringpb.ListTimeSeriesRequest_FULL,
	}

	tsIter := metricClient.ListTimeSeries(ctx, req)
	var sum float64
	var count int
	for {
		ts, err := tsIter.Next()
		if err == iterator.Done || err != nil {
			break
		}
		for _, point := range ts.GetPoints() {
			val := point.GetValue()
			if val.GetDoubleValue() != 0 {
				sum += val.GetDoubleValue() * 100.0
			} else {
				sum += float64(val.GetInt64Value()) * 100.0
			}
			count++
		}
	}
	if count == 0 {
		return 0.0
	}
	return sum / float64(count)
}
