# Use a Go version that matches the toolchain in go.mod
FROM golang:1.24

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

RUN go build -o openadserve ./cmd/server

EXPOSE 8787

CMD ["./openadserve"]
