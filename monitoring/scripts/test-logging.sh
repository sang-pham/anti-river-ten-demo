#!/bin/bash

# Test script for Go Demo API logging pipeline
# This script generates test requests to verify the monitoring setup

set -e

API_URL="${API_URL:-http://localhost:8081}"
KIBANA_URL="${KIBANA_URL:-http://localhost:5601}"
ELASTICSEARCH_URL="${ELASTICSEARCH_URL:-http://localhost:9200}"

echo "🧪 Testing Go Demo API logging pipeline..."
echo "API URL: $API_URL"
echo "Kibana URL: $KIBANA_URL"
echo "Elasticsearch URL: $ELASTICSEARCH_URL"

# Function to check if service is available
check_service() {
    local url=$1
    local service_name=$2
    
    if curl -s -f "$url" > /dev/null 2>&1; then
        echo "✅ $service_name is available"
        return 0
    else
        echo "❌ $service_name is not available at $url"
        return 1
    fi
}

# Function to generate test requests
generate_test_requests() {
    echo "📝 Generating test requests..."
    
    # Test health endpoints
    echo "Testing health endpoints..."
    curl -s "$API_URL/healthz" > /dev/null
    curl -s "$API_URL/readyz" > /dev/null
    
    # Test registration
    echo "Testing user registration..."
    REGISTER_RESPONSE=$(curl -s -X POST "$API_URL/v1/auth/register" \
        -H "Content-Type: application/json" \
        -d '{"username":"testuser","email":"test@example.com","password":"testpass123"}' \
        -w "%{http_code}")
    
    if [[ "$REGISTER_RESPONSE" == *"200"* ]] || [[ "$REGISTER_RESPONSE" == *"201"* ]]; then
        echo "✅ Registration successful"
    else
        echo "⚠️  Registration response: $REGISTER_RESPONSE"
    fi
    
    # Test login
    echo "Testing user login..."
    LOGIN_RESPONSE=$(curl -s -X POST "$API_URL/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d '{"identifier":"testuser","password":"testpass123"}')
    
    TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.token // empty' 2>/dev/null || echo "")
    
    if [ -n "$TOKEN" ] && [ "$TOKEN" != "null" ]; then
        echo "✅ Login successful, token obtained"
        
        # Test authenticated endpoint
        echo "Testing authenticated endpoint..."
        curl -s "$API_URL/v1/auth/me" \
            -H "Authorization: Bearer $TOKEN" > /dev/null
        echo "✅ Authenticated request completed"
    else
        echo "⚠️  Login failed or token not obtained"
    fi
    
    # Test error scenarios
    echo "Testing error scenarios..."
    
    # 404 error
    curl -s "$API_URL/nonexistent-endpoint" > /dev/null
    
    # 401 error
    curl -s "$API_URL/v1/auth/me" \
        -H "Authorization: Bearer invalid-token" > /dev/null
    
    # Invalid JSON
    curl -s -X POST "$API_URL/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d '{"invalid": json}' > /dev/null
    
    echo "✅ Error scenarios tested"
    
    # Generate some load
    echo "Generating load (10 requests)..."
    for i in {1..10}; do
        curl -s "$API_URL/healthz" > /dev/null &
    done
    wait
    echo "✅ Load generation completed"
}

# Function to check logs in Elasticsearch
check_elasticsearch_logs() {
    echo "🔍 Checking logs in Elasticsearch..."
    
    # Wait a moment for logs to be processed
    sleep 5
    
    # Check if indices exist
    INDICES=$(curl -s "$ELASTICSEARCH_URL/_cat/indices/go-demo-*?v" || echo "")
    
    if [ -n "$INDICES" ]; then
        echo "✅ Go Demo indices found:"
        echo "$INDICES"
    else
        echo "⚠️  No Go Demo indices found yet"
    fi
    
    # Check recent logs
    RECENT_LOGS=$(curl -s "$ELASTICSEARCH_URL/go-demo-*/_search?size=5&sort=@timestamp:desc" \
        -H "Content-Type: application/json" \
        -d '{"query":{"match_all":{}}}' | jq '.hits.total.value // 0' 2>/dev/null || echo "0")
    
    if [ "$RECENT_LOGS" -gt 0 ]; then
        echo "✅ Found $RECENT_LOGS recent log entries"
    else
        echo "⚠️  No recent log entries found"
    fi
}

# Function to check Kibana setup
check_kibana_setup() {
    echo "📊 Checking Kibana setup..."
    
    # Check if index patterns exist
    INDEX_PATTERNS=$(curl -s "$KIBANA_URL/api/saved_objects/_find?type=index-pattern" \
        -H "kbn-xsrf: true" | jq '.saved_objects | length' 2>/dev/null || echo "0")
    
    if [ "$INDEX_PATTERNS" -gt 0 ]; then
        echo "✅ Found $INDEX_PATTERNS index patterns in Kibana"
    else
        echo "⚠️  No index patterns found in Kibana"
    fi
    
    # Check if dashboards exist
    DASHBOARDS=$(curl -s "$KIBANA_URL/api/saved_objects/_find?type=dashboard" \
        -H "kbn-xsrf: true" | jq '.saved_objects | length' 2>/dev/null || echo "0")
    
    if [ "$DASHBOARDS" -gt 0 ]; then
        echo "✅ Found $DASHBOARDS dashboards in Kibana"
    else
        echo "⚠️  No dashboards found in Kibana"
    fi
}

# Function to display summary
display_summary() {
    echo ""
    echo "📋 Test Summary"
    echo "==============="
    echo ""
    echo "🔗 Access your monitoring:"
    echo "   Kibana Dashboard: $KIBANA_URL"
    echo "   Elasticsearch: $ELASTICSEARCH_URL"
    echo ""
    echo "📊 Recommended next steps:"
    echo "   1. Open Kibana and check the 'Go Demo API - Request/Response Monitoring' dashboard"
    echo "   2. Verify that logs are appearing in the Discover tab"
    echo "   3. Check that alerts are configured in Stack Management > Rules and Connectors"
    echo "   4. Generate more traffic to see real-time monitoring in action"
    echo ""
    echo "🔍 Troubleshooting:"
    echo "   - If no logs appear, check Fluentd logs: make monitoring-logs"
    echo "   - If services are down, restart: make monitoring-down && make monitoring-up"
    echo "   - For detailed setup, see: docs/monitoring.md"
}

# Main execution
main() {
    echo "🚀 Starting logging pipeline test..."
    echo ""
    
    # Check service availability
    echo "1️⃣  Checking service availability..."
    check_service "$API_URL/healthz" "Go Demo API"
    check_service "$ELASTICSEARCH_URL" "Elasticsearch"
    check_service "$KIBANA_URL/api/status" "Kibana"
    echo ""
    
    # Generate test requests
    echo "2️⃣  Generating test requests..."
    generate_test_requests
    echo ""
    
    # Check Elasticsearch logs
    echo "3️⃣  Checking Elasticsearch logs..."
    check_elasticsearch_logs
    echo ""
    
    # Check Kibana setup
    echo "4️⃣  Checking Kibana setup..."
    check_kibana_setup
    echo ""
    
    # Display summary
    display_summary
    
    echo "✅ Logging pipeline test completed!"
}

# Run main function
main "$@"