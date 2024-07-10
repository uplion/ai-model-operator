# Use the official Golang image to create a build artifact.
# This is based on Debian and sets the GOPATH to /go.
FROM golang:1.22 as builder

# Copy local code to the container image.
WORKDIR /app
COPY . .

RUN go mod download

# Build the binary.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app .

# Use a smaller base image to deploy the binary.
# This image is based on Alpine and doesn't include the Go runtime.
FROM alpine:3.14

# Copy the binary from the build stage.
COPY --from=builder /app/app /app

# Run the binary.
ENTRYPOINT ["/app"]
