FROM golang:1.12.0-alpine as builder
RUN apk add --no-cache bash git

ENV GOPROXY=https://goproxy.io
ENV GO111MODULE=on
WORKDIR ${GOPATH}/src/ushareit.com/spark-ui-controller-envoy/
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/bin/spark-ui-controller-envoy

FROM alpine
COPY --from=builder /usr/bin/spark-ui-controller-envoy /usr/bin
RUN apk add --no-cache tini

COPY entrypoint.sh /usr/bin/
ENTRYPOINT ["sh","/usr/bin/entrypoint.sh"]

