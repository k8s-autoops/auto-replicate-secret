FROM golang:1.14 AS builder
ENV CGO_ENABLED 0
WORKDIR /go/src/app
ADD . .
RUN go build -o /template-autoops-statefulset

FROM alpine:3.12
COPY --from=builder /template-autoops-statefulset /template-autoops-statefulset
CMD ["/template-autoops-statefulset"]