FROM golang:1.26.4-alpine AS builder

WORKDIR /app

COPY go.* ./ 
COPY third_party/gotgproto/* ./

RUN go mod download

COPY . .

RUN go build -o main main.go

FROM alpine:3.24.1

COPY --from=builder /app/main /app

RUN apk --no-cache add \
  bash \
  dumb-init

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD [ "/app" ]


