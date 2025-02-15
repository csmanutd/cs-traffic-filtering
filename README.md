# Traffic Filtering Tool

A tool suite for retrieving traffic data from CloudSecure and filtering based on IP lists.

## Features

- API Service (`api`):
  - Fetch traffic logs from CloudSecure via API
  - Segment data retrieval by time periods
  - Automatic retry mechanism for failed requests
  - Concurrent processing of time segments
  - S3 upload integration

- Filtering CLI (`filter_cli`):
  - Filter traffic data based on CIDR block lists
  - Support for multiple filtering conditions
  - Preset-based filtering configurations
  - Automatic S3 upload of filtered results
  - Command-line interface for automation

## Usage

### API Service (api)

1. Configure CloudSecure credentials:
   ```bash
   ./api -cs <cloudsecure_name>
   ```
   First-time users will be prompted to enter API credentials.

2. Fetch traffic data:
   ```bash
   ./api -out <output_file.csv> [--date YYYYMMDD] [--nos3]
   ```
   - Retrieves yesterday's data by default if no date is specified
   - `--nos3` flag skips S3 upload

### Filtering Tool (filter_cli)

1. List available presets:
   ```bash
   ./filter_cli --list-presets
   ```

2. Filter traffic data using a preset:
   ```bash
   ./filter_cli --input <input_file.csv> --preset <preset_name>
   ```

## Configuration Files

### cloudsecure.config
JSON file storing CloudSecure API configuration:
```json
{
    "api_key": "your_api_key",
    "api_secret": "your_api_secret",
    "tenant_id": "your_tenant_id"
}
```

### s3config.json
S3 upload configuration for both tools:
```json
{
    "preset_name": "default",
    "bucket_name": "your_bucket",
    "folder_name": "your_folder",
    "profile_name": "aws_profile",
    "region": "aws_region"
}
```

### presets.json
Filtering presets configuration:
```json
[
    {
        "name": "preset_name",
        "conditions": [
            {
                "field": "sourceIP",
                "operator": "==",
                "listFiles": ["subnets.txt"]
            }
        ],
        "flow_status": "ALLOWED"
    }
]
```

## Requirements

- Go 1.16+
- AWS CLI (for S3 uploads)
- Valid CloudSecure API credentials
- Configured AWS credentials (for S3 access)
