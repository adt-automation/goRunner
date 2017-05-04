# Get last commit ID
BUILD=`git rev-parse HEAD`

LDFLAGS=-ldflags "-s -w -X main.Build=${BUILD}"

build:
	go build ${LDFLAGS} 
	# go vet -v && go build ${LDFLAGS} 
