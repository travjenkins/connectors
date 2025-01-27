# Build Stage
################################################################################
FROM golang:1.17-bullseye as builder

WORKDIR /builder

# Download & compile dependencies early. Doing this separately allows for layer
# caching opportunities when no dependencies are updated.
COPY go.* ./
RUN go mod download

# Copy in a recent `flowctl` for usage by tests.
COPY --from=ghcr.io/estuary/flow:dev /usr/local/bin/flowctl /usr/local/bin/flowctl

# Build the connector projects we depend on.
COPY materialize-boilerplate ./materialize-boilerplate
COPY materialize-bigquery    ./materialize-bigquery
COPY testsupport             ./testsupport

# Test and build the connector.
RUN go test  -tags nozstd -v ./materialize-bigquery/...
RUN go build -tags nozstd -v -o ./connector ./materialize-bigquery/...

# Runtime Stage
################################################################################
FROM debian:bullseye-slim

RUN apt-get update -y \
 && apt-get install --no-install-recommends -y ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# Create a non-privileged "nonroot" user.
RUN useradd nonroot --create-home --shell /usr/sbin/nologin

# Avoid running the connector as root.
USER nonroot:nonroot

WORKDIR /connector
ENV PATH="/connector:$PATH"

# Bring in the compiled connector artifact from the builder.
COPY --from=builder /builder/connector ./connector

ENTRYPOINT ["/connector/connector"]
