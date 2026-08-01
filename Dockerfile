FROM golang:1.26-alpine AS builder
RUN apk add --no-cache make git
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make zqk-community

FROM alpine:latest
WORKDIR /app
COPY --from=builder /src/bin/zqk-community /usr/local/bin/zqk
ENTRYPOINT ["zqk"]
