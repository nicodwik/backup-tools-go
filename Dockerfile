# Stage 1: Build the Go application
# We use a specific version of Go on Alpine Linux for a smaller build environment.
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy your Go application's source code into the builder image.
# Assuming your compiled Go file is named 'main' or you will compile it to 'main'.
# If you have source code, you'd copy the source and build it here.
# For example, if your source is in the current directory:
COPY . .

# If your Go application has dependencies, download them.
# Uncomment the following line if you use go modules.
# RUN go mod download

# Build your Go application.
# CGO_ENABLED=0 is important for static linking, making the binary portable.
# GOOS=linux ensures it's compiled for Linux, the common OS in containers.
# -o myapp specifies the output binary name.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix netgo -o backup-tools-go .

# Stage 2: Create the final, minimal image
# We use a very small base image (alpine) for the final runtime.
# If your application has absolutely no external dependencies (e.g., no CGO, no SSL certs),
# you could even use FROM scratch, but alpine is generally safer for most Go apps.
FROM alpine:latest

# Install ca-certificates to handle HTTPS requests if your Go app makes them.
RUN apk add --no-cache ca-certificates

# Set the working directory inside the final image
WORKDIR /root/

# Copy the compiled binary from the builder stage into the final image
COPY --from=builder /app/backup-tools-go .
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /
ENV ZONEINFO=/zoneinfo.zip

# Define the command to run your application when the container starts.
# This assumes your compiled Go binary is named 'myapp'.
CMD ["./backup-tools-go"]
