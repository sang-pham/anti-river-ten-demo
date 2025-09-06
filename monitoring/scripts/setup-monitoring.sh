#!/bin/bash

# Setup script for Go Demo API monitoring infrastructure
# This script initializes Elasticsearch templates, ILM policies, and Kibana dashboards

set -e

ELASTICSEARCH_URL="${ELASTICSEARCH_URL:-http://localhost:9200}"
KIBANA_URL="${KIBANA_URL:-http://localhost:5601}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MONITORING_DIR="$(dirname "$SCRIPT_DIR")"

echo "🚀 Setting up Go Demo API monitoring infrastructure..."
echo "Elasticsearch URL: $ELASTICSEARCH_URL"
echo "Kibana URL: $KIBANA_URL"

# Function to wait for service to be ready
wait_for_service() {
    local url=$1
    local service_name=$2
    local max_attempts=30
    local attempt=1

    echo "⏳ Waiting for $service_name to be ready..."
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s -f "$url" > /dev/null 2>&1; then
            echo "✅ $service_name is ready!"
            return 0
        fi
        
        echo "   Attempt $attempt/$max_attempts - $service_name not ready yet..."
        sleep 10
        ((attempt++))
    done
    
    echo "❌ $service_name failed to start within expected time"
    return 1
}

# Function to create ILM policy
create_ilm_policy() {
    echo "📋 Creating ILM policy for log retention..."
    
    curl -X PUT "$ELASTICSEARCH_URL/_ilm/policy/go-demo-logs-policy" \
        -H "Content-Type: application/json" \
        -d @"$MONITORING_DIR/elasticsearch/ilm/go-demo-logs-policy.json" \
        -w "\nHTTP Status: %{http_code}\n"
    
    if [ $? -eq 0 ]; then
        echo "✅ ILM policy created successfully"
    else
        echo "❌ Failed to create ILM policy"
        return 1
    fi
}

# Function to create index template
create_index_template() {
    echo "📝 Creating index template for Go Demo logs..."
    
    curl -X PUT "$ELASTICSEARCH_URL/_index_template/go-demo-logs-template" \
        -H "Content-Type: application/json" \
        -d @"$MONITORING_DIR/elasticsearch/templates/go-demo-logs-template.json" \
        -w "\nHTTP Status: %{http_code}\n"
    
    if [ $? -eq 0 ]; then
        echo "✅ Index template created successfully"
    else
        echo "❌ Failed to create index template"
        return 1
    fi
}

# Function to import Kibana dashboards
import_kibana_dashboards() {
    echo "📊 Importing Kibana dashboards..."
    
    curl -X POST "$KIBANA_URL/api/saved_objects/_import" \
        -H "kbn-xsrf: true" \
        -H "Content-Type: application/json" \
        -d @"$MONITORING_DIR/kibana/dashboards/go-demo-request-monitoring.json" \
        -w "\nHTTP Status: %{http_code}\n"
    
    if [ $? -eq 0 ]; then
        echo "✅ Kibana dashboards imported successfully"
    else
        echo "❌ Failed to import Kibana dashboards"
        return 1
    fi
}

# Function to import Kibana alerts
import_kibana_alerts() {
    echo "🚨 Importing Kibana alerts..."
    
    curl -X POST "$KIBANA_URL/api/saved_objects/_import" \
        -H "kbn-xsrf: true" \
        -H "Content-Type: application/json" \
        -d @"$MONITORING_DIR/kibana/alerts/go-demo-alerts.json" \
        -w "\nHTTP Status: %{http_code}\n"
    
    if [ $? -eq 0 ]; then
        echo "✅ Kibana alerts imported successfully"
    else
        echo "❌ Failed to import Kibana alerts"
        return 1
    fi
}

# Function to create initial index
create_initial_index() {
    echo "🗂️  Creating initial log index..."
    
    curl -X PUT "$ELASTICSEARCH_URL/go-demo-general-$(date +%Y.%m.%d)-000001" \
        -H "Content-Type: application/json" \
        -d '{
            "aliases": {
                "go-demo-logs": {
                    "is_write_index": true
                }
            }
        }' \
        -w "\nHTTP Status: %{http_code}\n"
    
    if [ $? -eq 0 ]; then
        echo "✅ Initial index created successfully"
    else
        echo "❌ Failed to create initial index"
        return 1
    fi
}

# Main execution
main() {
    # Wait for services to be ready
    wait_for_service "$ELASTICSEARCH_URL" "Elasticsearch"
    wait_for_service "$KIBANA_URL/api/status" "Kibana"
    
    # Setup Elasticsearch components
    create_ilm_policy
    create_index_template
    create_initial_index
    
    # Setup Kibana components
    import_kibana_dashboards
    import_kibana_alerts
    
    echo ""
    echo "🎉 Monitoring setup completed successfully!"
    echo ""
    echo "📊 Access your monitoring dashboards:"
    echo "   Kibana: $KIBANA_URL"
    echo "   Elasticsearch: $ELASTICSEARCH_URL"
    echo ""
    echo "🔍 Default index patterns:"
    echo "   - go-demo-* (all logs)"
    echo "   - go-demo-access-* (access logs)"
    echo "   - go-demo-detailed-* (detailed request logs)"
    echo "   - go-demo-errors-* (error logs)"
    echo ""
    echo "📈 Key metrics to monitor:"
    echo "   - Request rate and response times"
    echo "   - Error rates by endpoint"
    echo "   - Slow requests (>1s)"
    echo "   - HTTP status code distribution"
    echo ""
    echo "🚨 Configured alerts:"
    echo "   - High error rate (>5%)"
    echo "   - Slow response times (95th percentile >2s)"
    echo "   - Server errors (≥5 5xx errors in 5min)"
    echo "   - High traffic (>1000 requests in 15min)"
    echo "   - Authentication failures (≥10 401s in 10min)"
}

# Run main function
main "$@"