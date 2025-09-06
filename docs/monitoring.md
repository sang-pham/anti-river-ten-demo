# Go Demo API - Kibana Monitoring Setup

This document provides comprehensive instructions for setting up Kibana monitoring for request/response logging in the Go Demo API project.

## Architecture Overview

The monitoring setup uses the ELK stack (Elasticsearch, Logstash, Kibana) with Fluentd for log collection:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Go API App    │───▶│     Fluentd     │───▶│ Elasticsearch   │
│  (JSON Logs)    │    │ (Log Collector) │    │  (Log Storage)  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                                                        │
                                               ┌─────────────────┐
                                               │     Kibana      │
                                               │  (Visualization)│
                                               └─────────────────┘
```

## Features

### Enhanced Logging

- **Structured JSON Output**: All logs are formatted as JSON for better parsing
- **Request/Response Tracking**: Detailed HTTP request and response logging
- **Request ID Correlation**: Each request gets a unique ID for tracing
- **Security**: Sensitive headers (Authorization, Cookie, etc.) are sanitized
- **Performance Metrics**: Response times, request sizes, and performance indicators

### Monitoring Capabilities

- **Real-time Dashboards**: Pre-built Kibana dashboards for API monitoring
- **Error Tracking**: Automatic error detection and alerting
- **Performance Analysis**: Response time percentiles and slow request identification
- **Traffic Analysis**: Request patterns, top endpoints, and user agent tracking
- **Log Retention**: Automated log lifecycle management with configurable retention

## Quick Start

### 1. Start the ELK Stack

```bash
# Navigate to monitoring directory
cd monitoring

# Start Elasticsearch, Kibana, and Fluentd
docker-compose -f docker-compose.elk.yml up -d

# Wait for services to be ready (this may take a few minutes)
docker-compose -f docker-compose.elk.yml logs -f
```

### 2. Initialize Monitoring Infrastructure

```bash
# Run the setup script to configure Elasticsearch and Kibana
./scripts/setup-monitoring.sh
```

### 3. Start Your Application

```bash
# Navigate back to project root
cd ..

# Copy environment configuration
cp .env.example .env

# Start the application with Fluentd logging
docker-compose up -d
```

### 4. Access Monitoring Dashboards

- **Kibana**: http://localhost:5601
- **Elasticsearch**: http://localhost:9200

## Configuration Details

### Environment Variables

Add these variables to your `.env` file:

```bash
# Application version for logging context
APP_VERSION=1.0.0

# Fluentd configuration
FLUENTD_HOST=host.docker.internal
FLUENTD_PORT=24224
```

### Log Structure

The enhanced logging provides structured JSON output with the following fields:

#### Basic Fields

- `@timestamp`: ISO 8601 timestamp
- `service`: Service name (go-demo-api)
- `version`: Application version
- `environment`: Environment (development/staging/production)
- `level`: Log level (info, warn, error, debug)
- `msg`: Log message
- `request_id`: Unique request identifier

#### HTTP Request Fields

- `http.method`: HTTP method (GET, POST, etc.)
- `http.url`: Full request URL
- `http.path`: Request path
- `http.query`: Query parameters
- `http.protocol`: HTTP protocol version
- `http.scheme`: URL scheme (http/https)
- `http.host`: Host header
- `http.remote_addr`: Client IP address
- `http.user_agent`: User agent string
- `http.referer`: Referer header
- `http.content_length`: Request content length
- `http.content_type`: Request content type

#### HTTP Response Fields

- `http.status_code`: HTTP status code
- `http.status_class`: Status class (2xx, 3xx, 4xx, 5xx)
- `http.response_size_bytes`: Response size in bytes

#### Performance Fields

- `duration_ms`: Request duration in milliseconds
- `duration_ns`: Request duration in nanoseconds
- `is_error`: Boolean indicating if request resulted in error (4xx/5xx)
- `is_slow`: Boolean indicating if request took >1 second

#### Request/Response Data

- `request.headers`: Sanitized request headers
- `request.body`: Truncated request body (configurable limit)
- `response.body`: Truncated response body (configurable limit)

### Index Patterns

The setup creates the following Elasticsearch indices:

- `go-demo-access-*`: Basic HTTP access logs
- `go-demo-detailed-*`: Detailed request/response logs
- `go-demo-errors-*`: Error logs (4xx/5xx responses)
- `go-demo-general-*`: General application logs

### Log Retention Policy

The Index Lifecycle Management (ILM) policy automatically manages log retention:

- **Hot Phase**: Active indices, up to 1GB or 1 day
- **Warm Phase**: After 2 days, optimized for search
- **Cold Phase**: After 7 days, minimal resources
- **Delete Phase**: After 30 days, logs are deleted

## Kibana Dashboards

### Go Demo API - Request/Response Monitoring

The main dashboard includes:

1. **Requests Over Time**: Time series of request volume
2. **HTTP Status Code Distribution**: Pie chart of response status codes
3. **Response Time Percentiles**: 50th, 95th, and 99th percentile response times
4. **Error Rate**: Percentage of requests resulting in errors
5. **Top API Endpoints**: Most frequently accessed endpoints with metrics
6. **Slow Requests**: Requests taking longer than 1 second
7. **Top User Agents**: Most common user agents accessing the API

### Creating Custom Dashboards

1. Navigate to Kibana at http://localhost:5601
2. Go to **Management** → **Stack Management** → **Index Patterns**
3. Create index pattern for `go-demo-*`
4. Use **Discover** to explore your logs
5. Create visualizations in **Visualize Library**
6. Combine visualizations into dashboards

## Alerting and Monitoring

### Key Metrics to Monitor

1. **Error Rate**: Percentage of 4xx/5xx responses
2. **Response Time**: 95th percentile response time
3. **Request Volume**: Requests per minute/hour
4. **Slow Requests**: Requests taking >1 second
5. **Failed Requests**: 5xx server errors

### Setting Up Alerts

You can set up alerts in Kibana for:

- Error rate exceeding threshold (e.g., >5%)
- Response time degradation (e.g., 95th percentile >2s)
- High volume of 5xx errors
- Unusual traffic patterns

## Troubleshooting

### Common Issues

#### Network Conflicts

```bash
# If you get "Pool overlaps with other one on this address space" error
make monitoring-clean  # Clean up resources and networks
make monitoring-up     # Try starting again

# Or manually clean up
docker network prune -f
docker volume prune -f
```

#### Fluentd Connection Issues

```bash
# Check if Fluentd is running
docker-compose -f monitoring/docker-compose.elk.yml ps fluentd

# Check Fluentd logs
docker-compose -f monitoring/docker-compose.elk.yml logs fluentd

# Test Fluentd connectivity
curl -X POST -d 'json={"message":"test"}' http://localhost:9880/test
```

#### Elasticsearch Issues

```bash
# Check Elasticsearch health
curl http://localhost:9200/_cluster/health

# Check indices
curl http://localhost:9200/_cat/indices?v

# Check index templates
curl http://localhost:9200/_index_template
```

#### Kibana Issues

```bash
# Check Kibana status
curl http://localhost:5601/api/status

# Check Kibana logs
docker-compose -f monitoring/docker-compose.elk.yml logs kibana
```

### Log Levels

Adjust log levels using the `LOG_LEVEL` environment variable:

- `debug`: Detailed debugging information
- `info`: General information (default)
- `warn`: Warning messages
- `error`: Error messages only

### Performance Tuning

#### For High-Volume Applications

1. **Increase Fluentd Buffer Size**:

   ```yaml
   # In monitoring/fluentd/conf/fluent.conf
   <buffer>
   chunk_limit_size 8M
   queue_limit_length 16
   </buffer>
   ```

2. **Adjust Elasticsearch Settings**:

   ```yaml
   # In monitoring/elasticsearch/config/elasticsearch.yml
   indices.memory.index_buffer_size: 20%
   index.refresh_interval: 30s
   ```

3. **Optimize Log Truncation**:
   ```bash
   # Reduce body capture size
   MAX_BODY_BYTES=1024
   ```

## Security Considerations

### Production Deployment

1. **Enable Elasticsearch Security**:

   ```yaml
   xpack.security.enabled: true
   xpack.security.http.ssl.enabled: true
   ```

2. **Use Strong Passwords**:

   ```bash
   # Generate passwords for Elasticsearch users
   docker exec -it elasticsearch bin/elasticsearch-setup-passwords auto
   ```

3. **Network Security**:

   - Use private networks for ELK communication
   - Implement firewall rules
   - Use TLS/SSL for all connections

4. **Log Sanitization**:
   - Ensure sensitive data is not logged
   - Review and update header sanitization rules
   - Consider data masking for PII

### Data Privacy

The logging system automatically sanitizes sensitive headers:

- `Authorization`
- `Cookie`
- `Set-Cookie`
- `X-API-Key`

Review and extend this list based on your security requirements.

## Maintenance

### Regular Tasks

1. **Monitor Disk Usage**: Elasticsearch indices can grow large
2. **Review ILM Policies**: Adjust retention based on requirements
3. **Update Dashboards**: Add new visualizations as needed
4. **Performance Monitoring**: Monitor ELK stack performance
5. **Security Updates**: Keep ELK stack versions updated

### Backup and Recovery

```bash
# Create Elasticsearch snapshot repository
curl -X PUT "localhost:9200/_snapshot/backup_repository" -H 'Content-Type: application/json' -d'
{
  "type": "fs",
  "settings": {
    "location": "/usr/share/elasticsearch/backup"
  }
}'

# Create snapshot
curl -X PUT "localhost:9200/_snapshot/backup_repository/snapshot_1"
```

## Support and Resources

- [Elasticsearch Documentation](https://www.elastic.co/guide/en/elasticsearch/reference/current/index.html)
- [Kibana Documentation](https://www.elastic.co/guide/en/kibana/current/index.html)
- [Fluentd Documentation](https://docs.fluentd.org/)
- [Go slog Package](https://pkg.go.dev/log/slog)

For project-specific issues, check the application logs and monitoring dashboards for insights.
