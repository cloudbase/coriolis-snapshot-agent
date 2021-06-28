SHELL := bash

.PHONY : build-static

IMAGE_TAG = coriolis-snapshot-agent

build-static:
	docker build --tag $(IMAGE_TAG) .
	docker run --rm -v $(PWD):/build/coriolis-snapshot-agent $(IMAGE_TAG) /build-static.sh
