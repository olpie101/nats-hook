FROM golang:1.14.1-alpine as builder

ENV GO111MODULE="on"
WORKDIR /go/src/github.com/olpie101/hookbridge
COPY go.mod .
COPY go.sum .
COPY main.go .

RUN GOOS=linux CGO_ENABLED=0 go build -a -o hookbridge main.go


FROM alpine:3.11

RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /go/src/github.com/olpie101/hookbridge .
ENTRYPOINT ["/root/hookbridge"]