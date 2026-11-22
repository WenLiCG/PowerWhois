FROM alpine:3.24.4
RUN apk add --no-cache ca-certificates
WORKDIR /typonamer
USER 1000:1000
ENTRYPOINT ["/typonamer/typonamer"]
