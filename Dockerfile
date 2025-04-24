FROM golang:1.24.2-alpine3.21

WORKDIR /usr/src/links

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY main.go .
RUN go build -v -o /usr/local/bin/links .

COPY . .

CMD ["/usr/local/bin/links", "--links=/usr/src/links/links.json"]
