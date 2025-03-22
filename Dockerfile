FROM golang:1.24.1 AS builder

WORKDIR /app

RUN go mod init exec-annotator

RUN go get -d -v ./...

COPY . .  
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o webhook .

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/webhook .
COPY certs/cert.pem /etc/webhook/certs/tls.crt
COPY certs/key.pem /etc/webhook/certs/tls.key

CMD ["./webhook"]
