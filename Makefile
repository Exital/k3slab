.PHONY: test test-lab test-build run run-build

IMAGE ?= k3slab-tests
RUN_IMAGE ?= k3slab:latest
CONTAINER ?= k3slab
LAB ?= 01-kubectl-basics
DOCKERFILE := docker/Dockerfile
REPORT_VOL := k3slab-test-reports

run-build:
	docker build -f $(DOCKERFILE) -t $(RUN_IMAGE) .

run: run-build
	docker run --rm --name $(CONTAINER) \
		--privileged \
		--cgroupns=host \
		-p 3010:3010 \
		-p 80:80 \
		-e LAB_ID=$(LAB) \
		-e k9s_enable=true \
		$(RUN_IMAGE)

test-build:
	docker build -f $(DOCKERFILE) --target tests -t $(IMAGE) .

test: test-build
	docker run --rm --privileged --cgroupns=host \
		-v "$(CURDIR)/lab:/src/lab:ro" \
		-e K3SLAB_TEST_REPORT_DIR=/reports \
		-v $(REPORT_VOL):/reports \
		$(IMAGE)

test-lab: test-build
	@if [ -z "$(LAB)" ]; then echo "usage: make test-lab LAB=01-kubectl-basics"; exit 1; fi
	docker run --rm --privileged --cgroupns=host \
		-v "$(CURDIR)/lab:/src/lab:ro" \
		-e K3SLAB_TEST_ONLY=lab-e2e \
		-e K3SLAB_TEST_LAB=$(LAB) \
		-e K3SLAB_TEST_REPORT_DIR=/reports \
		-v $(REPORT_VOL):/reports \
		$(IMAGE)
