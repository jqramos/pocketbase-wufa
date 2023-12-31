FROM golang:1.20-alpine as gogcc
ENV GOOS=linux
ENV GOARCH=amd64
ENV CGO_ENABLED=1

RUN apk update && apk add --no-cache \
        gcc \
        musl-dev

# Build the binary
FROM gogcc as builder

WORKDIR /app

# Download dependencies
COPY go.mod .
COPY go.sum .
RUN go mod download && go mod verify


# Build /app/bin
# COPY internal ./internal
COPY migrations ./migrations
COPY wufa/main.go .
COPY wufa_api/ ./wufa_api
COPY wufa_core/ ./wufa_core

RUN go build -ldflags="-s -w" -o bin -v ./main.go

# Serve the binary with pb_public
FROM alpine:latest as bin

RUN apk update && apk add --no-cache \
        gcc \
        musl-dev

WORKDIR /app/
# COPY pb_data ./pb_data
COPY --from=builder /app/bin .

EXPOSE 8090

CMD ["/app/bin", "serve", "--http=0.0.0.0:8090"]