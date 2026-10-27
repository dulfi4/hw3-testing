FROM golang:1.25.6-alpine3.23@sha256:98e6cffc31ccc44c7c15d83df1d69891efee8115a5bb7ede2bf30a38af3e3c92

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY *_test.go ./

CMD ["go", "test", "-count=1", "-v", "-timeout=3m", "./..."]
