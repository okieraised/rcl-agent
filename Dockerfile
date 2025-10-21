FROM golang:1.25-alpine3.22 AS build

ENV APP_NAME=rcl-agent

COPY vg.pem  /etc/ssl/certs/ca-certificates.crt
COPY vg.pem  /usr/local/share/ca-certificates/vg.crt

RUN apk update && apk add --no-cache ca-certificates
RUN update-ca-certificates

RUN mkdir /opt/app
WORKDIR /opt/app

COPY go.mod go.sum ./
RUN go mod download

COPY ./*.go ./
COPY ./docs ./docs
COPY ./internal ./internal
COPY ./server ./server

#RUN go install golang.org/x/vuln/cmd/govulncheck@latest
#RUN govulncheck ./...
#
#RUN go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
#RUN fieldalignment -json -fix ./...

WORKDIR /opt/app

RUN go build -ldflags="-s -w" -o ${APP_NAME} .

FROM alpine:latest

ENV APP_NAME=rcl-agent

RUN mkdir /opt/app
WORKDIR  /opt/app

COPY vg.pem  /etc/ssl/certs/vg-certificates.crt
COPY vg.pem  /usr/local/share/ca-certificates/vg.crt

RUN apk update && apk add --no-cache ffmpeg tzdata ca-certificates
RUN update-ca-certificates

ENV TZ=Asia/Ho_Chi_Minh
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

COPY --from=build /opt/app/$APP_NAME /opt/app/$APP_NAME
RUN mkdir ./conf
RUN touch ./conf/config.toml

CMD ["./rcl-agent"]