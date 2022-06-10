# Dockerfile to clone the goresponsiveness repository
# build the binary
# make it ready to run

# Build with: docker build -t goresp .

# Run with: docker run --rm goresp

FROM golang:1.18.3-alpine3.16

RUN mkdir /goresponsiveness
ADD . /goresponsiveness
WORKDIR /goresponsiveness

RUN go mod download
RUN go build -o networkQuality networkQuality.go 

# `docker run` invokes the networkQuality binary that was just built
ENTRYPOINT ["/goresponsiveness/networkQuality"]

# These default parameters test against Apple's public servers
# If you change any of these on the `docker run` command, you need to provide them all
CMD ["-config","mensura.cdn-apple.com","-port","443","-path","/api/v1/gm/config"]

