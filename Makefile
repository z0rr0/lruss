PROGRAM=LRUSS
BIN=bin
VERSION=`bash version.sh`
ROOTPKG=github.com/z0rr0/lruss
SOURCEDIR=$(GOPATH)/src/$(ROOTPKG)


all: test

install:
	go install -ldflags "$(VERSION)" $(ROOTPKG)

run: install
	cp -f $(SOURCEDIR)/config.example.json config.json
	$(GOPATH)/$(BIN)/lruss

lint: install
	go vet $(ROOTPKG)/trim
	golint $(ROOTPKG)/trim

test: lint
	# go tool cover -html=coverage.out
	# go tool trace ratest.test trace.out
	go test -race -v -cover -coverprofile=trim_coverage.out -trace trim_trace.out $(ROOTPKG)/trim
	go test -race -v -cover -coverprofile=conf_coverage.out -trace conf_trace.out $(ROOTPKG)/conf

bench: test
	go test -bench=. -benchmem -v $(ROOTPKG)/trim

arm:
	env GOOS=linux GOARCH=arm go install -ldflags "$(VERSION)" $(ROOTPKG)

linux:
	env GOOS=linux GOARCH=amd64 go install -ldflags "$(VERSION)" $(ROOTPKG)

clean:
	rm -rf $(GOPATH)/$(BIN)/*
	find $(SOURCEDIR) -type f -name "*.out" -delete

