#!/bin/bash

# Initialize Elasticsearch with templates and policies for FluentD
set -e

ELASTICSEARCH_URL="${ELASTICSEARCH_URL:-http://elasticsearch:9200}"
MAX_RETRIES=30
RETRY_INTERVAL=10

echo "🔧 Initializing Elasticsearch for FluentD logging..."

# Function to wait for Elasticsearch to be ready
wait_for_elasticsearch() {
    local attempt=1
    echo "⏳ Waiting for Elasticsearch to be ready..."
    
    while [ $attempt -le $MAX_RETRIES ]; do
        if curl -s -f "$ELASTICSEARCH_URL/_cluster/health" > /dev/null 2>&1; then
            echo "✅ Elasticsearch is ready!"
            return 0
        fi
        
        echo "   Attempt $attempt/$MAX_RETRIES - Elasticsearch not ready yet..."
        sleep $RETRY_INTERVAL
        ((attempt++))
    done
    
    echo "❌ Elasticsearch failed to start within expected time"
    return 1
}

# Function to create ILM policy
create_ilm_policy() {
    echo "📋 Creating ILM policy..."
    
    curl -X PUT "$ELASTICSEARCH_URL/_ilm/policy/go-demo-logs-policy" \
        -H "Content-Type: application/json" \
        -d '{
            "policy": {
                "phases": {
                    "hot": {
                        "min_age": "0ms",
                        "actions": {
                            "rollover": {
                                "max_size": "1gb",
                                "max_age": "1d",
                                "max_docs": 1000000
                            },
                            "set_priority": {
                                "priority": 100
                            }
                        }
                    },
                    "warm": {
                        "min_age": "2d",
                        "actions": {
                            "set_priority": {
                                "priority": 50
                            },
                            "allocate": {
                                "number_of_replicas": 0
                            },
                            "forcemerge": {
                                "max_num_segments": 1
                            }
                        }
                    },
                    "cold": {
                        "min_age": "7d",
                        "actions": {
                            "set_priority": {
                                "priority": 0
                            },
                            "allocate": {
                                "number_of_replicas": 0
                            }
                        }
                    },
                    "delete": {
                        "min_age": "30d",
                        "actions": {
                            "delete": {}
                        }
                    }
                }
            }
        }' \
        -w "\nHTTP Status: %{http_code}\n"
    
    echo "✅ ILM policy created"
}

# Function to create index template
create_index_template() {
    echo "📝 Creating index template..."
    
    curl -X PUT "$ELASTICSEARCH_URL/_index_template/go-demo-logs-template" \
        -H "Content-Type: application/json" \
        -d @/fluentd/etc/go-demo-logs-template.json \
        -w "\nHTTP Status: %{http_code}\n"
    
    echo "✅ Index template created"
}

# Function to create initial indices
create_initial_indices() {
    echo "🗂️  Creating initial indices..."
    
    local date_suffix=$(date +%Y.%m.%d)
    local indices=("go-demo-access" "go-demo-detailed" "go-demo-errors" "go-demo-general")
    
    for index in "${indices[@]}"; do
        echo "Creating index: ${index}-${date_suffix}-000001"
        curl -X PUT "$ELASTICSEARCH_URL/${index}-${date_suffix}-000001" \
            -H "Content-Type: application/json" \
            -d "{
                \"aliases\": {
                    \"${index}\": {
                        \"is_write_index\": true
                    }
                }
            }" \
            -w "\nHTTP Status: %{http_code}\n"
    done
    
    echo "✅ Initial indices created"
}

# Main execution
main() {
    wait_for_elasticsearch
    create_ilm_policy
    create_index_template
    create_initial_indices
    
    echo "🎉 Elasticsearch initialization completed successfully!"
}

# Run main function
main "$@"