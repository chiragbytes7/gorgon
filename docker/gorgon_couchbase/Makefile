MODULE_FILES = $(shell echo ../../src/*/go.mod ../../src/*/go.sum)

all: compose.yaml control/src.tgz node/src.tgz control/deps.tgz node/deps.tgz node/override.tgz

control/src.tgz: node/src.tgz
	cp $< $@

control/deps.tgz: node/deps.tgz
	cp $< $@

node/src.tgz: $(shell find ../../src)
	tar -czf $@ -C ../.. src

node/deps.tgz: $(MODULE_FILES)
	tar -czf $@ -C ../.. $(MODULE_FILES:../../%=%)

compose.yaml: compose.control.yaml compose.node.yaml compose.py
	python3 compose.py 3

build: all
	docker compose -f compose.yaml build

node/override.tgz : $(shell find ../../override)
	tar -czf $@ -C ../.. override 

clean:
	rm */*.tgz
