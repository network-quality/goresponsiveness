PKG          := github.com/network-quality/goresponsiveness
GIT_VERSION  := $(shell git describe --always --long)
LDFLAGS      := -ldflags "-X $(PKG)/utilities.GitVersion=$(GIT_VERSION)"

all: build test
build:
	go build $(LDFLAGS) networkQuality.go
test:
	go test ./timeoutat/ ./traceable/ ./utilities/ ./lgc ./qualityattenuation ./rpm ./series
golines:
	find . -name '*.go' -exec ~/go/bin/golines -w {} \;
clean:
	go clean -testcache
	rm -f *.o core
