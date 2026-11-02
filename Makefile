GO ?= go
CURL ?= curl
UNZIP ?= unzip

NTA_CACHE ?= .cache/nta-jis
NTA_ZIP := $(NTA_CACHE)/syukutaimap1_0_0.zip
NTA_XLSX := $(NTA_CACHE)/jissyukutaimap1_0_0.xlsx
NTA_ZIP_SHA256 := f4c6073aa036a0c29347de76d9ac47efbdea986ef65f163984c2bec694e4af9b
NTA_XLSX_SHA256 := 61973084e775593964d2bc3358c223bb01481f1b6555634a420682a2d649405b
NTA_URL := https://www.houjin-bangou.nta.go.jp/download/images/syukutaimap1_0_0.zip
PHP_SRC ?= ../php2go/third_party/php-src
PHP_REVISION ?= $(shell git -C $(PHP_SRC) rev-parse HEAD 2>/dev/null)
CC ?= cc

.PHONY: nta-jis-ucm
nta-jis-ucm: $(NTA_XLSX)
	$(GO) run ./cmd/ntajis -input $(NTA_XLSX) -sha256 $(NTA_XLSX_SHA256)
	$(GO) run ./cmd/genmbcswide

$(NTA_ZIP):
	@mkdir -p $(NTA_CACHE)
	@set -eu; trap 'rm -f "$@.tmp"' EXIT; \
		$(CURL) -fL -A 'Mozilla/5.0' -o "$@.tmp" $(NTA_URL); \
		printf '%s  %s\n' $(NTA_ZIP_SHA256) "$@.tmp" | sha256sum -c -; \
		mv "$@.tmp" "$@"

.PHONY: php-mbstring-ucm
php-mbstring-ucm:
	@test -f "$(PHP_SRC)/main/php_config.h" || { echo 'PHP_SRC must point to a configured php-src checkout' >&2; exit 1; }
	@test -n "$(PHP_REVISION)" || { echo 'Set PHP_REVISION to the php-src revision' >&2; exit 1; }
	@mkdir -p .cache/php-mbstring
	$(CC) -D_GNU_SOURCE -O2 -ffunction-sections -fdata-sections \
		-I$(PHP_SRC) -I$(PHP_SRC)/main -I$(PHP_SRC)/Zend -I$(PHP_SRC)/TSRM \
		-I$(PHP_SRC)/ext/mbstring/libmbfl -I$(PHP_SRC)/ext/mbstring/libmbfl/mbfl \
		cmd/phpmbucm/testdata/probe.c -Wl,--gc-sections -o .cache/php-mbstring/probe
	$(GO) run ./cmd/phpmbucm -probe .cache/php-mbstring/probe -revision $(PHP_REVISION)
	@printf '%s\n' '$(PHP_REVISION)' > data/php-mbstring/UPSTREAM_REVISION
	@cd data/php-mbstring && LC_ALL=C sha256sum *.ucm > ucm.sha256
	$(GO) run ./cmd/gensbcs
	$(GO) run ./cmd/genmbcswide

$(NTA_XLSX): $(NTA_ZIP)
	@set -eu; trap 'rm -f "$@.tmp"' EXIT; \
		$(UNZIP) -p "$<" jissyukutaimap1_0_0.xlsx > "$@.tmp"; \
		printf '%s  %s\n' $(NTA_XLSX_SHA256) "$@.tmp" | sha256sum -c -; \
		mv "$@.tmp" "$@"
