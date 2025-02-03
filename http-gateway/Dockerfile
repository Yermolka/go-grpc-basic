FROM golang

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY http-gateway/ .
COPY proto/presence/ proto/presence/
COPY proto/ proto/
COPY templates/ templates/
COPY static/ static/

RUN go build -o /app/http-gateway

CMD ["/app/http-gateway"]
