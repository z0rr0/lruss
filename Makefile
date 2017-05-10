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
	go vet $(ROOTPKG)/conf
	golint $(ROOTPKG)/conf
	go vet $(ROOTPKG)/web
	golint $(ROOTPKG)/web
	go vet $(ROOTPKG)/admin
	golint $(ROOTPKG)/admin
	golint $(ROOTPKG)
	go vet $(ROOTPKG)

test: lint
	# go tool cover -html=coverage.out
	# go tool trace ratest.test trace.out
#	go test -race -v -cover -coverprofile=trim_coverage.out -trace trim_trace.out $(ROOTPKG)/trim
#	go test -race -v -cover -coverprofile=conf_coverage.out -trace conf_trace.out $(ROOTPKG)/conf
#	go test -race -v -cover -coverprofile=web_coverage.out -trace web_trace.out $(ROOTPKG)/web
	go test -race -v -cover -coverprofile=admin_coverage.out -trace admin_trace.out $(ROOTPKG)/admin

bench: lint
	go test -bench=. -benchmem -v $(ROOTPKG)/trim
	go test -bench=. -benchmem -v $(ROOTPKG)/admin

arm:
	env GOOS=linux GOARCH=arm go install -ldflags "$(VERSION)" $(ROOTPKG)

linux:
	env GOOS=linux GOARCH=amd64 go install -ldflags "$(VERSION)" $(ROOTPKG)

clean:
	rm -rf $(GOPATH)/$(BIN)/*
	find $(SOURCEDIR) -type f -name "*.out" -delete

