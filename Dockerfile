FROM golang:1.25

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -o service ./cmd/

EXPOSE 80 443 22

CMD [ "./service" ]
