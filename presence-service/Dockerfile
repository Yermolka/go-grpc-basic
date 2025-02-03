FROM golang

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY presence-service/ .
COPY proto/presence/ proto/presence/
COPY proto/ proto/

RUN go build -o /app/presence-service

CMD ["/app/presence-service"]
