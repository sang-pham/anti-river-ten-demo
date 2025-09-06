FROM fluent/fluentd:v1.16-debian-1

USER root

# Install curl for health checks and initialization
RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*

# Install the plugin compatible with ES8
RUN gem install fluent-plugin-elasticsearch -v 5.4.3 --no-document \
  && gem install elasticsearch -v 8.15.0 --no-document \
  && gem install elasticsearch-transport -v 8.15.0 --no-document

# Copy initialization script
COPY conf/init-elasticsearch.sh /usr/local/bin/init-elasticsearch.sh
RUN chmod +x /usr/local/bin/init-elasticsearch.sh

USER fluent
