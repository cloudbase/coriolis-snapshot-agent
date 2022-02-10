SHELL := bash

.PHONY : build-static setup-veeamsnap install clean fresh

BIN_PATH = /usr/local/bin/coriolis-snapshot-agent
IMAGE_TAG = coriolis-snapshot-agent
VEEAMSNAP_IN_MODULES = $(shell grep "veeamsnap" /etc/modules)
VEEAMSNAP_REPO_URL = https://github.com/veeam/veeamsnap
TMP = /tmp
UNAME = $(shell uname -r)

build-static:
	docker build --tag $(IMAGE_TAG) .
	docker run --rm -v $(PWD):/build/coriolis-snapshot-agent $(IMAGE_TAG) /build-static.sh

setup-veeamsnap:
ifeq (,$(wildcard $(TMP)/veeamsnap))
	cd $(TMP); git clone $(VEEAMSNAP_REPO_URL)
endif
ifeq (,$(wildcard /lib/modules/$(UNAME)/kernel/drivers/veeam/veeamsnap.ko))
	@echo Building veeamsnap module
	cd $(TMP)/veeamsnap/source; make; make install
endif
	@echo Setting up veeamsnap module
ifeq ($(VEEAMSNAP_IN_MODULES),)
	echo veeamsnap >> /etc/modules
endif
	touch /etc/udev/rules.d/99-veeamsnap.rules
	echo 'KERNEL=="veeamsnap", OWNER="root", GROUP="disk"' > /etc/udev/rules.d/99-veeamsnap.rules
	modprobe veeamsnap

install: setup-veeamsnap build-static
	@echo Copying agent binary
	cp $(PWD)/coriolis-snapshot-agent $(BIN_PATH)
	@echo Copying configuration and service unit files
	mkdir -p /etc/coriolis-snapshot-agent/certs
	mkdir -p /mnt/snapstores/snapstore_files
ifeq (,$(wildcard /etc/coriolis-snapshot-agent/config.toml))
	cp $(PWD)/contrib/config.toml.sample /etc/coriolis-snapshot-agent/config.toml
endif
ifeq (,$(wildcard /etc/systemd/system/coriolis-snapshot-agent.service))
	cp $(PWD)/contrib/coriolis-snapshot-agent.service.sample /etc/systemd/system/coriolis-snapshot-agent.service
	systemctl daemon-reload
endif
	@echo Creating coriolis user
	useradd --system --home-dir=/nonexisting --groups disk --no-create-home --shell /bin/false coriolis
	chown coriolis:disk -R /etc/coriolis-snapshot-agent
	chown coriolis:disk -R /mnt/snapstores
	@echo DONE!

clean:
	@echo Disabling snapshot agent service
	systemctl disable --now coriolis-snapshot-agent
	@echo Removing coriolis user
	userdel coriolis
	@echo Removing Coriolis Snapshot Agent
	rm $(BIN_PATH)
	rmmod veeamsnap

fresh: clean install
