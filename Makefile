all: build test
build:
	go build networkQuality.go
test:
	go test ./timeoutat/ ./traceable/ ./ms/ ./utilities/
golines:
	find . -name '*.go' -exec ~/go/bin/golines -w {} \;
clean:
	go clean -testcache
	rm -f *.o core
