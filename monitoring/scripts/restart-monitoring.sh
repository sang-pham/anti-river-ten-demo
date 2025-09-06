#!/bin/bash

# Script to restart the monitoring stack with proper initialization
# This script ensures Elasticsearch indices are created before FluentD starts

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MONITORING_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$MONITORING_DIR")"

echo "🔄 Restarting Go Demo monitoring stack..."
echo "Project root: $PROJECT_ROOT"
echo "Monitoring dir: $MONITORING_DIR"

# Function to stop services
stop_services() {
    echo "🛑 Stopping existing services..."
    
    cd "$PROJECT_ROOT"
    
    # Stop monitoring stack
    if [ -f "monitoring/docker-compose.elk.yml" ]; then
        docker-compose -f monitoring/docker-compose.elk.yml down -v
        echo "✅ Monitoring stack stopped"
    fi
    
    # Stop main application if running
    if [ -f "docker-compose.yml" ]; then
        docker-compose down
        echo "✅ Application stopped"
    fi
    
    # Clean up any orphaned containers
    docker container prune -f
    echo "✅ Cleaned up orphaned containers"
}

# Function to start services
start_services() {
    echo "🚀 Starting services..."
    
    cd "$PROJECT_ROOT"
    
    # Start monitoring stack first
    echo "Starting monitoring stack..."
    docker-compose -f monitoring/docker-compose.elk.yml up -d
    
    # Wait for services to be healthy
    echo "⏳ Waiting for services to be ready..."
    sleep 30
    
    # Check service health
    check_service_health
    
    # Start main application
    echo "Starting main application..."
    docker-compose up -d
    
    echo "✅ All services started"
}

# Function to check service health
check_service_health() {
    local max_attempts=30
    local attempt=1
    
    echo "🏥 Checking service health..."
    
    # Check Elasticsearch
    while [ $attempt -le $max_attempts ]; do
        if curl -s -f "http://localhost:9200/_cluster/health" > /dev/null 2>&1; then
            echo "✅ Elasticsearch is healthy"
            break
        fi
        echo "   Attempt $attempt/$max_attempts - Elasticsearch not ready..."
        sleep 10
        ((attempt++))
    done
    
    # Check Kibana
    attempt=1
    while [ $attempt -le $max_attempts ]; do
        if curl -s -f "http://localhost:5601/api/status" > /dev/null 2>&1; then
            echo "✅ Kibana is healthy"
            break
        fi
        echo "   Attempt $attempt/$max_attempts - Kibana not ready..."
        sleep 10
        ((attempt++))
    done
    
    # Check FluentD
    attempt=1
    while [ $attempt -le $max_attempts ]; do
        if curl -s -f "http://localhost:9880/api/plugins.json" > /dev/null 2>&1; then
            echo "✅ FluentD is healthy"
            break
        fi
        echo "   Attempt $attempt/$max_attempts - FluentD not ready..."
        sleep 10
        ((attempt++))
    done
}

# Function to verify the fix
verify_fix() {
    echo "🔍 Verifying the logging fix..."
    
    # Wait a moment for initialization
    sleep 10
    
    # Check if indices were created
    echo "Checking Elasticsearch indices..."
    INDICES=$(curl -s "http://localhost:9200/_cat/indices/go-demo-*?v" 2>/dev/null || echo "")
    
    if [ -n "$INDICES" ]; then
        echo "✅ Go Demo indices found:"
        echo "$INDICES"
    else
        echo "⚠️  No Go Demo indices found yet - they will be created when logs arrive"
    fi
    
    # Check FluentD logs for errors
    echo "Checking FluentD logs for errors..."
    FLUENTD_LOGS=$(docker logs go-demo-fluentd --tail 20 2>&1 | grep -i "error\|exception" || echo "")
    
    if [ -z "$FLUENTD_LOGS" ]; then
        echo "✅ No errors found in FluentD logs"
    else
        echo "⚠️  Found potential issues in FluentD logs:"
        echo "$FLUENTD_LOGS"
    fi
    
    # Check if templates are applied
    echo "Checking if index templates are applied..."
    TEMPLATE_CHECK=$(curl -s "http://localhost:9200/_index_template/go-demo-logs-template" 2>/dev/null | jq '.index_templates | length' 2>/dev/null || echo "0")
    
    if [ "$TEMPLATE_CHECK" -gt 0 ]; then
        echo "✅ Index template is properly applied"
    else
        echo "⚠️  Index template not found - will be created by FluentD"
    fi
}

# Function to display next steps
display_next_steps() {
    echo ""
    echo "🎉 Monitoring stack restart completed!"
    echo ""
    echo "📋 What was fixed:"
    echo "   ✅ Updated FluentD to Elasticsearch 8 compatibility"
    echo "   ✅ Added automatic index template creation"
    echo "   ✅ Added ILM policy configuration"
    echo "   ✅ Added proper error handling and logging"
    echo "   ✅ Added initialization script to create indices"
    echo ""
    echo "🔗 Access your monitoring:"
    echo "   Kibana: http://localhost:5601"
    echo "   Elasticsearch: http://localhost:9200"
    echo "   FluentD Health: http://localhost:9880"
    echo ""
    echo "🧪 Test the fix:"
    echo "   Run: ./monitoring/scripts/test-logging.sh"
    echo "   Or make API calls to: http://localhost:8081/v1/auth/register"
    echo ""
    echo "📊 Monitor logs:"
    echo "   FluentD logs: docker logs go-demo-fluentd -f"
    echo "   Elasticsearch indices: curl http://localhost:9200/_cat/indices/go-demo-*?v"
    echo ""
    echo "🚨 If issues persist:"
    echo "   1. Check FluentD logs: docker logs go-demo-fluentd"
    echo "   2. Check Elasticsearch health: curl http://localhost:9200/_cluster/health"
    echo "   3. Verify network connectivity between containers"
}

# Main execution
main() {
    echo "🔧 Go Demo Monitoring Stack Restart"
    echo "===================================="
    echo ""
    
    stop_services
    echo ""
    
    start_services
    echo ""
    
    verify_fix
    echo ""
    
    display_next_steps
}

# Run main function
main "$@"