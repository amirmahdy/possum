# Possum: FinOps Cloud Resource Optimizer

**Possum** is a pluggable, multi-cloud FinOps utility designed to scan AWS and GCP environments to discover, analyze, and report unused or underutilized resources. The goal is to provide engineering and finance teams with clear visibility into wasted spend, concrete remediation recommendations, and safe automated or semi-automated cleanup options.

---

## 1. High-Level Architecture

Possum uses a streamlined, scanner-centric architecture. Rather than introducing intermediate cloud-specific adapters, the Core Orchestrator leverages a unified Cloud Adapters credential module to instantiate and inject sessions directly into cloud-specific resource scanners.

```mermaid
graph TD
    A[CLI / Orchestrator] --> B[Configuration Manager]
    A --> C[Cloud Adapters / Credentials]
    A --> D[Rules Engine]
    A --> E[Reporting Engine]
    
    C --> |Injected SDK Sessions/Clients| S1[AWS Scanners]
    C --> |Injected SDK Sessions/Clients| S2[GCP Scanners]
    
    S1 --> |Raw Cloud Data| D
    S2 --> |Raw Cloud Data| D
    
    D --> |Evaluated & Tagged Violations| E
    E --> R1[JSON/CSV Output]
    E --> R2[Console Summary]
    E --> R3[Interactive HTML Dashboard]
    E --> R4[Slack / Email Notifications]
```

---

## 2. Software Components Breakdown

### 2.1 Core Engine (Orchestrator)
The **Core Engine** acts as the central coordinator of the system.
* **Responsibilities**:
  * Resolves configuration settings and triggers the configuration manager.
  * Uses the **Cloud Adapters** library to establish active sessions/clients.
  * Injects sessions/clients directly into resource scanners and manages scanner execution concurrency.
  * Aggregates findings from different scanners and routes them to the Rules Engine.

### 2.2 Configuration Manager
The **Configuration Manager** parses and validates user-defined scan options.
* **Responsibilities**:
  * Parses configurations (e.g., `possum.yaml`).
  * Manages target accounts, GCP projects, regions, and globally excluded resources/tags.

### 2.3 Cloud Adapters (Session & Credentials Provider)
This component manages AWS and GCP client initialization, removing the need for separate wrapper classes for each cloud provider.
* **Responsibilities**:
  * Resolves authentication for AWS (credentials file, profile, IAM role assumption) and GCP (Service Account keys, Application Default Credentials).
  * Returns authenticated SDK client instances (e.g. `boto3.Session` or raw GCP client contexts).
  * Directly injects these clients/sessions into the active scanners during orchestrator initialization.

### 2.4 Resource Scanners (Pluggable Modules)
Scanners are light, single-purpose worker classes that implement a standard interface (e.g., `scan(client, context) -> List[RawResource]`). They receive their authenticated clients directly from the Core Engine.

#### AWS Scanners:
1. **`UnattachedEBSVolumeScanner`**: Finds EBS volumes in the `available` state (not attached to any EC2 instance).
2. **`UnassociatedElasticIPScanner`**: Detects Elastic IP addresses (EIPs) allocated but not mapped to an active instance or network interface.
3. **`IdleEC2Scanner`**: Queries CloudWatch CPU utilization and network metrics over a set lookback window.
4. **`IdleRDSScanner`**: Evaluates CloudWatch connection metrics (`DatabaseConnections == 0`) and CPU usage.
5. **`UnusedLoadBalancerScanner`**: Identifies ALBs, NLBs, and Classic ELBs with zero active targets or requests.

#### GCP Scanners:
1. **`UnattachedDiskScanner`**: Locates Compute Engine Persistent Disks (PDs) that are not attached to any VM.
2. **`OrphanedStaticIPScanner`**: Identifies reserved external IP addresses that are not associated with any virtual machines.
3. **`IdleVMScanner`**: Evaluates Cloud Monitoring CPU utilization and network metrics.
4. **`IdleCloudSQLScanner`**: Detects Cloud SQL instances with no connections and negligible CPU usage.

### 2.5 Rules & Thresholds Engine
The **Rules Engine** processes raw resource data collected by scanners and evaluates them against composite optimization rules.
* **Responsibilities**:
  * Evaluates multi-metric historical usage over a 30-day lookback window (CPU Average, CPU 95th Percentile P95, Network I/O transfer MB, and Disk IOPS).
  * Enforces composite idleness conditions (e.g. `CPU Avg < 2%` AND `CPU P95 < 5%` AND `Network < 500MB`) to eliminate false positives from periodic cron jobs or network gateways.
  * Filters resources containing excluded tags (e.g. `env: production`).
  * Calculates estimated monthly waste based on pricing lookups.

### 2.6 Reporting & Notification Engine
Formats scanning results and delivers them to stakeholders.
* **Responsibilities**:
  * Formats results for console tables, JSON/CSV files, or interactive HTML dashboards.
  * Sends Slack/Email notifications to resource owners based on tags.

---

## 3. Data Flow and Sequence

The execution flow of a single run represents how Possum transitions from startup to reporting:

```mermaid
sequenceDiagram
    autonumber
    actor User as User / Cron Job
    participant Core as Core Engine
    participant Auth as Cloud Adapters
    participant Scan as Scanners
    participant Rules as Rules Engine
    participant Rep as Reporting Engine

    User->>Core: Trigger Scan (possum scan --config possum.yaml)
    Core->>Core: Load and Validate Configuration
    Core->>Auth: Request Cloud Sessions/Clients (AWS / GCP)
    Auth-->>Core: Injected SDK Clients / Sessions
    Core->>Scan: Run Scanners (passing injected Clients)
    Scan->>Scan: Query Cloud APIs directly using clients
    Scan-->>Core: List of raw candidate resources
    Core->>Rules: Pass raw data to Rules Engine
    Rules->>Rules: Filter by Tag Exclusions & Idle Thresholds
    Rules->>Rules: Calculate estimated monthly cost savings
    Rules-->>Core: Validated waste recommendations
    Core->>Rep: Send structured recommendations
    Rep->>Rep: Render Outputs (JSON, HTML, Console, Slack)
    Rep-->>User: Return results summary / Write report files
```

---

## 4. Unified Data Schema

To make output consumption consistent, all scanner results are normalized into a unified JSON format:

```json
{
  "resource_id": "arn:aws:ec2:us-east-1:123456789012:volume/vol-0abcd1234efgh5678",
  "name": "production-log-volume-temp",
  "cloud_provider": "aws",
  "account_id_or_project": "123456789012",
  "service": "ec2:volume",
  "region_or_zone": "us-east-1a",
  "status": "available",
  "tags": {
    "env": "staging",
    "owner": "data-team",
    "project": "log-aggregation"
  },
  "waste_metrics": {
    "reason": "Unattached for 12 days",
    "last_activity": "2026-07-09T04:12:00Z",
    "metric_summary": {
      "ops_per_second": 0,
      "bytes_read_write": 0
    }
  },
  "cost_metrics": {
    "estimated_monthly_cost": 48.50,
    "size_gb": 400,
    "currency": "USD"
  },
  "remediation": {
    "recommended_action": "SNAPSHOT_AND_DELETE",
    "risk_level": "LOW",
    "impact_description": "Safe to delete once a snapshot is taken because it is unattached."
  }
}
```

---

## 5. Sample Configuration File (`possum.yaml`)

Users configure Possum's behavior using a YAML configuration file. Below is a sample configuration showing how to adjust thresholds, list target regions/projects, and define exclusions.

```yaml
version: "1.0"

# Global exclusions based on resource tag matching
global_exclusions:
  tags:
    env: "production"
    protection: "do-not-delete"
    finops: "ignore"
  resource_ids:
    - "arn:aws:ec2:us-east-1:123456789012:volume/vol-0123456789abcdef0"

# Configuration for cloud providers
providers:
  aws:
    enabled: true
    regions:
      - us-east-1
      - us-west-2
      - eu-west-1
    auth:
      type: "profile"
      profile_name: "finops-scanner"
      # Alternative: role_arn: "arn:aws:iam::123456789012:role/PossumFinopsRole"
    thresholds:
      idle_ec2:
        cpu_percent_max: 2.0
        cpu_p95_max: 5.0
        network_in_out_mb_max: 500.0
        lookback_days: 30
      idle_rds:
        connections_max: 0
        lookback_days: 30

  gcp:
    enabled: true
    projects:
      - "billing-staging-1029"
      - "data-science-sandbox"
    auth:
      type: "credentials_file"
      file_path: "/secrets/gcp-sa-key.json"
    thresholds:
      idle_vm:
        cpu_percent_max: 2.0
        cpu_p95_max: 5.0
        network_in_out_mb_max: 500.0
        lookback_days: 30
      idle_cloud_sql:
        connections_max: 0
        lookback_days: 30

# Reporting outputs configurations
reporting:
  console:
    enabled: true
    verbose: false
  json:
    enabled: true
    output_path: "./reports/possum-report.json"
  html:
    enabled: true
    output_path: "./reports/possum-dashboard.html"
  slack:
    enabled: false
    webhook_url: "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX"
    notify_channel: "#finops-alerts"
