# Get last commit ID
BUILD=`git rev-parse HEAD`

LDFLAGS=-ldflags "-s -w -X github.com/adt-automation/goRunner.Build=${BUILD}"

build:
	go vet -v && go build ${LDFLAGS} 
