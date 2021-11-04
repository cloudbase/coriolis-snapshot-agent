FROM golang:alpine

WORKDIR /root

RUN apk add musl-dev gcc libtool m4 autoconf g++ make libblkid util-linux-dev git linux-headers

RUN git clone https://github.com/veeam/veeamsnap /tmp/veeamsnap
RUN mkdir -p /usr/src/
RUN cp -a /tmp/veeamsnap/source /usr/src/veeamsnap-5.0.0.0

ADD ./scripts/build-static.sh /build-static.sh
RUN chmod +x /build-static.sh

CMD ["/bin/sh"]
