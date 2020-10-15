FROM golang:1.14 AS builder
ENV CGO_ENABLED 0
WORKDIR /go/src/app
ADD . .
RUN go build -o /auto-replicate-secret

FROM alpine:3.12
COPY --from=builder /auto-replicate-secret /auto-replicate-secret
CMD ["/auto-replicate-secret"]