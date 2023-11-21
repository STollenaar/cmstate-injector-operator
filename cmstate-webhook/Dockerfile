FROM golang:1.21.1-alpine as build
ENV GO111MODULE on
ENV CGO_ENABLED 0

RUN apk add git make openssl gpgme-dev libassuan-dev

WORKDIR /go/src/pod-mutating-wh
ADD . .

RUN go build -o podMutatingWH

FROM alpine
WORKDIR /app
COPY --from=build /go/src/pod-mutating-wh/podMutatingWH .
CMD [ "/app/podMutatingWH" ]