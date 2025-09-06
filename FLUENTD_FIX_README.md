# FluentD Logging Fix - Index Not Found Exception

## Problem Description

The FluentD service was experiencing `index_not_found_exception` errors when trying to write logs to Elasticsearch. This occurred because:

1. FluentD was configured for Elasticsearch 7 but the stack was running Elasticsearch 8
2. Elasticsearch indices were not being created automatically
3. Index templates and ILM policies were not properly applied by FluentD
4. Missing initialization sequence to ensure Elasticsearch was ready

## Root Cause Analysis

The error logs showed:

```
2025-09-05 17:27:43 +0000 [debug]: #0 fluent/log.rb:341:debug: Indexed (op = index), 6 index_not_found_exception
```

This happened because:

- FluentD tried to write to indices like `go-demo-access-2025.09.05` that didn't exist
- The Elasticsearch plugin version was incompatible with ES8
- No proper initialization sequence was in place

## Solution Implemented

### 1. Updated FluentD Configuration (`monitoring/fluentd/conf/fluent.conf`)

**Changes made:**

- Updated `default_elasticsearch_version` from `7` to `8`
- Added proper template configuration:
  ```yaml
  template_name go-demo-logs-template
  template_file /fluentd/etc/go-demo-logs-template.json
  template_overwrite true
  ```
- Added ILM policy configuration:
  ```yaml
  ilm_policy go-demo-logs-policy
  enable_ilm true
  ilm_policy_overwrite true
  ```
- Added better error handling and logging options

### 2. Updated FluentD Dockerfile (`monitoring/fluentd/Dockerfile`)

**Changes made:**

- Updated Elasticsearch plugin to version `5.4.3` (ES8 compatible)
- Updated Elasticsearch gem to version `8.15.0`
- Added curl for health checks and initialization
- Added initialization script

### 3. Created Initialization Script (`monitoring/fluentd/conf/init-elasticsearch.sh`)

**Purpose:**

- Waits for Elasticsearch to be ready
- Creates ILM policies automatically
- Creates index templates
- Creates initial indices with proper aliases
- Runs before FluentD starts processing logs

### 4. Updated Docker Compose (`monitoring/docker-compose.elk.yml`)

**Changes made:**

- Changed from pre-built image to custom build
- Added initialization command that runs before FluentD starts
- Added proper environment variables
- Ensured proper startup sequence

### 5. Created Management Scripts

- **`monitoring/scripts/restart-monitoring.sh`**: Complete restart with verification
- **`monitoring/scripts/test-logging.sh`**: Test the logging pipeline
- **`monitoring/scripts/setup-monitoring.sh`**: Initial setup (existing, made executable)

## How to Apply the Fix

### Option 1: Use the Restart Script (Recommended)

```bash
# Navigate to project root
cd /path/to/go-demo

# Run the restart script
./monitoring/scripts/restart-monitoring.sh
```

### Option 2: Manual Steps

```bash
# Stop existing services
docker-compose -f monitoring/docker-compose.elk.yml down -v
docker-compose down

# Rebuild FluentD with new configuration
docker-compose -f monitoring/docker-compose.elk.yml build fluentd

# Start services
docker-compose -f monitoring/docker-compose.elk.yml up -d

# Wait for services to be ready (30-60 seconds)
sleep 60

# Start main application
docker-compose up -d
```

## Verification Steps

### 1. Check Service Health

```bash
# Check Elasticsearch
curl http://localhost:9200/_cluster/health

# Check FluentD
curl http://localhost:9880/api/plugins.json

# Check Kibana
curl http://localhost:5601/api/status
```

### 2. Verify Index Creation

```bash
# List all go-demo indices
curl http://localhost:9200/_cat/indices/go-demo-*?v

# Check index templates
curl http://localhost:9200/_index_template/go-demo-logs-template
```

### 3. Test Logging Pipeline

```bash
# Run the test script
./monitoring/scripts/test-logging.sh

# Or manually test API endpoints
curl -X POST http://localhost:8081/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","email":"test@example.com","password":"testpass123"}'
```

### 4. Check Logs in Elasticsearch

```bash
# Search for recent logs
curl -X GET "http://localhost:9200/go-demo-*/_search?size=5&sort=@timestamp:desc" \
  -H "Content-Type: application/json" \
  -d '{"query":{"match_all":{}}}'
```

## Monitoring and Troubleshooting

### Check FluentD Logs

```bash
# View FluentD logs
docker logs go-demo-fluentd -f

# Check for errors
docker logs go-demo-fluentd 2>&1 | grep -i "error\|exception"
```

### Check Elasticsearch Health

```bash
# Cluster health
curl http://localhost:9200/_cluster/health?pretty

# Node info
curl http://localhost:9200/_nodes/stats?pretty

# Index health
curl http://localhost:9200/_cat/indices?v
```

### Common Issues and Solutions

#### Issue: FluentD still shows index_not_found_exception

**Solution:**

1. Ensure the initialization script ran successfully
2. Check if indices exist: `curl http://localhost:9200/_cat/indices/go-demo-*?v`
3. Restart FluentD: `docker restart go-demo-fluentd`

#### Issue: No logs appearing in Elasticsearch

**Solution:**

1. Check if the application is generating logs
2. Verify FluentD is receiving logs: `docker logs go-demo-fluentd | grep "fluent.info"`
3. Check network connectivity between containers

#### Issue: Template not applied

**Solution:**

1. Manually apply template: `./monitoring/scripts/setup-monitoring.sh`
2. Check template exists: `curl http://localhost:9200/_index_template/go-demo-logs-template`

## File Changes Summary

### Modified Files:

- `monitoring/fluentd/conf/fluent.conf` - Updated ES8 compatibility and template config
- `monitoring/fluentd/Dockerfile` - Updated plugins and added initialization
- `monitoring/docker-compose.elk.yml` - Added build context and initialization command

### New Files:

- `monitoring/fluentd/conf/go-demo-logs-template.json` - Index template for FluentD
- `monitoring/fluentd/conf/init-elasticsearch.sh` - Elasticsearch initialization script
- `monitoring/scripts/restart-monitoring.sh` - Complete restart script
- `FLUENTD_FIX_README.md` - This documentation

### Made Executable:

- `monitoring/scripts/restart-monitoring.sh`
- `monitoring/scripts/test-logging.sh`
- `monitoring/scripts/setup-monitoring.sh`
- `monitoring/fluentd/conf/init-elasticsearch.sh`

## Expected Results After Fix

1. ✅ No more `index_not_found_exception` errors in FluentD logs
2. ✅ Automatic creation of Elasticsearch indices when logs arrive
3. ✅ Proper application of index templates and ILM policies
4. ✅ Logs visible in Kibana Discover tab
5. ✅ Dashboards showing real-time metrics
6. ✅ Proper log rotation and retention via ILM policies

## Next Steps

1. **Test the fix**: Run `./monitoring/scripts/test-logging.sh`
2. **Generate traffic**: Make API calls to `/v1/auth/register` and other endpoints
3. **Monitor dashboards**: Open Kibana at http://localhost:5601
4. **Set up alerts**: Configure monitoring alerts in Kibana
5. **Performance tuning**: Adjust FluentD buffer settings if needed

## Support

If you encounter any issues after applying this fix:

1. Check the troubleshooting section above
2. Review FluentD logs: `docker logs go-demo-fluentd`
3. Verify all services are healthy using the verification steps
4. Run the restart script again if needed

The fix addresses the core issue of index creation and ES8 compatibility, ensuring reliable log ingestion from your Go application to the ELK stack.
