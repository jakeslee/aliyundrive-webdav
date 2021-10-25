#FROM golang:1.17.1-stretch as builder
#
#ADD . /go/src
#
#WORKDIR /go/src
#
## 禁用 CGO，避免 so 问题
#ENV CGO_ENABLED=0
#RUN go build -v -o /go/bin/aliyundrive-webdav main.go

FROM alpine:3.14.0
LABEL maintainers="Jakes Lee"
LABEL description="阿里云盘 WebDAV 桥接服务"

EXPOSE 8081

#COPY --from=builder /go/bin/aliyundrive-webdav /aliyundrive-webdav

ADD ./aliyundrive-webdav /aliyundrive-webdav

RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime &&\
    echo 'Asia/Shanghai' >/etc/timezone

RUN chmod +x /aliyundrive-webdav
CMD ["/aliyundrive-webdav"]
