.PHONY: test test-lab test-build

IMAGE ?= k3slab-tests
DOCKERFILE := docker/Dockerfile
REPORT_VOL := k3slab-test-reports

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
