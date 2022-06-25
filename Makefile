dest?=$(HOME)/bin
p=dmake
.PHONY: all clean install
all:; @go build -o $p && go vet
test:; @go test
clean:;	@rm -f $p
install: all; install -c -m 555 $p $(dest)
