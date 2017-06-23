# Get last commit ID
BUILD=`git rev-parse HEAD`

LDFLAGS=-ldflags "-s -w -X main.Build=${BUILD}"

build:
	go build ${LDFLAGS} 
	# go vet -v

build-linux:
	GOARCH=amd64 GOOS=linux CGO_ENABLED=0 go build ${LDFLAGS}
