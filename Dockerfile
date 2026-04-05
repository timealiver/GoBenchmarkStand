FROM golang:bookworm

ENV GOTOOLCHAIN=auto

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go run ./bench/gen -out bench/payloads

CMD ["go", "test", "./bench/", "-bench=BenchmarkHandlers", "-benchmem", "-count=10", "-timeout=900s"]
