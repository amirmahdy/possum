# Possum: FinOps Cloud Resource Scanner (Go CLI)

Possum is a multi-cloud FinOps utility designed to identify unused or underutilized resources in cloud environments. This project contains standalone Go CLI utilities for scanning AWS and GCP environments.

---

## Prerequisites & Installation

To install dependencies and prepare the tools for execution, run:
```bash
go mod tidy
```

### Running Unit Tests

To run unit tests across all packages (`pricing`, `aws` scanner, and `gcp` scanner):
```bash
go test -v ./...
```

### Authentication Setup

1. **AWS Configuration**:
   Configure credentials via the AWS CLI:
   ```bash
   aws configure
   ```

2. **GCP Configuration**:
   Establish Application Default Credentials (ADC) using the Google Cloud SDK:
   ```bash
   gcloud auth application-default login
   ```
   Alternatively, you can provide the path to a service account JSON credentials key using CLI arguments.

---

## Architectural Components & CLI Usage

Possum's CLI operations align directly with the software components described in `components.md`.

### 1. Core Engine (Orchestrator)
The Core Engine coordinates scans across cloud providers using a pluggable `Scanner` interface (`pkg/scanner`).
* **Component Details**: See Core Engine Section in `components.md`.
* **Execution Commands**:
  * Run a default AWS scan:
    ```bash
    go run main.go -provider aws
    ```
  * Run a default GCP scan:
    ```bash
    go run main.go -provider gcp -project my-gcp-project-id
    ```
  * Run a multi-cloud scan (AWS + GCP):
    ```bash
    go run main.go -provider all -project my-gcp-project-id
    ```

### 2. Cloud Adapters (Session & Credentials Provider)
This component initializes and manages cloud session credentials and passes them directly to active scanners.
* **Component Details**: See Cloud Adapters Section in `components.md`.
* **AWS Execution Commands**:
  * Run using a specific AWS credentials profile:
    ```bash
    go run main.go -provider aws -profile staging-profile
    ```
  * Scan a specific region:
    ```bash
    go run main.go -provider aws -region us-west-2
    ```
* **GCP Execution Commands**:
  * Run using a path to a service account JSON file:
    ```bash
    go run main.go -provider gcp -project my-project -credentials /path/to/sa-key.json
    ```

### 3. Resource Scanners
These are pluggable worker tasks implementing the `scanner.Scanner` interface (`pkg/scanner/scanner.go`).
* **Component Details**: See Resource Scanners Section in `components.md`.
* **AWS Scanners (`pkg/scanner/aws/aws.go`)**:
  * EBS Scanner: Queries volumes in the `available` (unattached) state and calculates monthly savings using volume type-specific rates (`gp2`, `gp3`, `io1`, `io2`, `st1`, `sc1`, `standard`).
  * EIP Scanner: Queries Elastic IPs lacking an `AssociationId`.
  * EC2 Scanner: Triggers CloudWatch checks for CPU utilization (`Avg` & `P95`), total network transfer (MB), and total disk IOPS.
* **GCP Scanners (`pkg/scanner/gcp/gcp.go`)**:
  * Persistent Disk Scanner: Queries disks in the `READY` state that have no entries in their `users` list.
  * Static IP Scanner: Queries regional/global IP allocations in the `RESERVED` status.
  * VM Scanner: Queries GCP Cloud Monitoring API for running instances' CPU utilization (`Avg` & `P95`) and actual network byte transfer (`sent_bytes_count` + `received_bytes_count`).

### 4. Rules & Thresholds Engine
This component filters candidate resources and evaluates whether they meet the composite thresholds configured for "idleness."
* **Component Details**: See Rules & Thresholds Section in `components.md`.
* **Available Threshold Flags**:
  * `-cpu-threshold`: CPU % average threshold (default: `2.0%`)
  * `-cpu-p95-threshold`: CPU % 95th percentile threshold (default: `5.0%`)
  * `-network-mb-threshold`: Total network transfer MB threshold (default: `500.0 MB`)
  * `-disk-ops-threshold`: Total disk IOPS threshold over lookback window (default: `1000.0 ops`)
  * `-lookback-days`: Lookback window in days (default: `30` days)
* **AWS Execution Commands**:
  * Query idle EC2 instances with CPU Avg < `2.0%`, CPU P95 < `5.0%`, Network < `500 MB`, and Disk Ops < `1000` over a `30`-day window:
    ```bash
    go run main.go -provider aws -cpu-threshold 2.0 -cpu-p95-threshold 5.0 -network-mb-threshold 500 -disk-ops-threshold 1000 -lookback-days 30
    ```
* **GCP Execution Commands**:
  * Query idle GCP VMs with CPU Avg < `2.0%`, CPU P95 < `5.0%`, and Network < `500 MB` over a `30`-day window:
    ```bash
    go run main.go -provider gcp -project my-project -cpu-threshold 2.0 -cpu-p95-threshold 5.0 -network-mb-threshold 500 -lookback-days 30
    ```

### 5. Reporting & Notification Engine
Aggregates findings across scanners and prints structured output to the terminal, summarizing estimated monthly waste.
* **Component Details**: See Reporting & Notification Section in `components.md`.
* **Typical GCP Output Example**:
  ```text
  ============================================================
  Starting Possum GCP Scan in project: billing-staging-1029
  ============================================================

  Scanning for Unattached Persistent Disks...
  Found 1 unattached Persistent Disks.

  Scanning for Unassociated Static IPs...
  Found 1 unassociated Static IPs.

  Scanning for Idle VMs (Avg CPU < 2.00%, P95 CPU < 5.00%, Net < 500.00MB over 30 days)...
  Found 1 idle VM instances.

  ============================================================
  POSSUM GCP WASTE ANALYSIS REPORT
  ============================================================

  [1] Persistent Disk (pd-balanced): logs-temp-disk
      Details:   Zone: us-central1-a, Size: 100 GB, Status: READY (Unattached)
      Labels:    map[env:staging]
      Est. Cost: $10.00/month

  [2] Static IP Address: reserved-ip-service
      Details:   Region: us-central1, IP Address: 35.184.21.19
      Labels:    None
      Est. Cost: $2.92/month

  [3] Idle VM (e2-medium): legacy-db-migration
      Details:   Zone: us-central1-b, Avg CPU: 0.45%, P95 CPU: 1.20%, Net Transfer: 12.40 MB over 30 days, Status: RUNNING
      Labels:    map[env:staging owner:db-admin]
      Est. Cost: $24.46/month

  ------------------------------------------------------------
  TOTAL POTENTIAL SAVINGS: $37.38 / month
  ------------------------------------------------------------
  ```
