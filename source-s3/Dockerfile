# Build Stage
################################################################################
FROM golang:1.17-buster as builder

WORKDIR /builder

# Download & compile dependencies early. Doing this separately allows for layer
# caching opportunities when no dependencies are updated.
COPY go.* ./
RUN go mod download

# Build the connector projects we depend on.
COPY parser/*.go ./parser/
COPY filesource ./filesource
COPY source-s3 ./source-s3

# Run the unit tests.
RUN go test -v ./parser/...
RUN go test -v ./filesource/...
RUN go test -v ./source-s3/...

# Build the connector.
RUN go build -o ./connector -v ./source-s3/...


# Runtime Stage
################################################################################
FROM gcr.io/distroless/base-debian10

WORKDIR /connector
ENV PATH="/connector:$PATH"

# Grab the statically-built parser cli.
COPY parser/target/x86_64-unknown-linux-musl/release/parser ./parser

# Bring in the compiled connector artifact from the builder.
COPY --from=builder /builder/connector ./connector

# Avoid running the connector as root.
USER nonroot:nonroot

ENTRYPOINT ["/connector/connector"]
