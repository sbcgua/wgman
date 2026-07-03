BINARY := wgman
GO ?= go
INSTALL ?= install

PREFIX ?= /usr/local
SBINDIR ?= $(PREFIX)/sbin
CONFIG_DIR ?= /etc/wireguard/wgman
CONFIG_SRC_DIR := share/etc/wireguard/wgman

.PHONY: all build test vet fmt check install install-bin install-config clean

all: build

build:
	$(GO) build -trimpath -ldflags="-s -w" -o $(BINARY) .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

check: fmt test vet

install: install-bin install-config

install-bin: build
	$(INSTALL) -d "$(DESTDIR)$(SBINDIR)"
	$(INSTALL) -m 0755 "$(BINARY)" "$(DESTDIR)$(SBINDIR)/$(BINARY)"

install-config:
	$(INSTALL) -d -m 0750 "$(DESTDIR)$(CONFIG_DIR)"
	@for f in config.yaml db.yaml user.conf.template; do \
		src="$(CONFIG_SRC_DIR)/$$f"; \
		dst="$(DESTDIR)$(CONFIG_DIR)/$$f"; \
		if [ -e "$$dst" ]; then \
			echo "keeping existing $$dst"; \
		else \
			echo "installing $$dst"; \
			$(INSTALL) -m 0640 "$$src" "$$dst"; \
		fi; \
	done

clean:
	rm -f "$(BINARY)" "$(BINARY).exe" coverage.out coverage.html
