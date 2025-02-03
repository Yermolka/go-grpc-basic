FROM golang

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY auth-service/ .
COPY proto/presence/ proto/presence/
COPY proto/ proto/

RUN go build -o /app/auth-service

CMD ["/app/auth-service"]
