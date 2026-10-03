FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /vpn-api .

FROM alpine:3.22
RUN apk add --no-cache wireguard-tools iproute2
COPY --from=build /vpn-api /vpn-api

EXPOSE 8080
CMD ["/vpn-api"]
