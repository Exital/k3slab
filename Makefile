.PHONY: test test-lab test-build test-light run run-build

IMAGE ?= k3slab-tests
RUN_IMAGE ?= k3slab:latest
CONTAINER ?= k3slab
LAB ?= 01-kubectl-basics
INGRESS_HOST ?= localhost
STUDENT_USERNAME ?= student
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
		-e K3SLAB_INGRESS_HOST=$(INGRESS_HOST) \
		-e student_username=$(STUDENT_USERNAME) \
		-e k9s_enable=true \
		$(RUN_IMAGE)

test-build:
	docker build -f $(DOCKERFILE) --target tests -t $(IMAGE) .

# Light phases only (unit / integration / frontend) in one container.
test-light: test-build
	docker run --rm --privileged --cgroupns=host \
		-v "$(CURDIR)/lab:/src/lab:ro" \
		-e K3SLAB_TEST_ONLY=backend-unit,backend-integration,frontend-build \
		-e K3SLAB_TEST_REPORT_DIR=/reports \
		-v $(REPORT_VOL):/reports \
		$(IMAGE)

# Full local suite: light once, then one fresh container per lab (sequential).
test: test-build
	IMAGE=$(IMAGE) REPORT_VOL=$(REPORT_VOL) bash docker/run-host-tests.sh

test-lab: test-build
	@if [ -z "$(LAB)" ]; then echo "usage: make test-lab LAB=01-kubectl-basics"; exit 1; fi
	docker run --rm --privileged --cgroupns=host \
		-p 80:80 \
		-v "$(CURDIR)/lab:/src/lab:ro" \
		-e K3SLAB_TEST_ONLY=lab-e2e \
		-e K3SLAB_TEST_LAB=$(LAB) \
		-e K3SLAB_TEST_REPORT_DIR=/reports \
		-v $(REPORT_VOL):/reports \
		$(IMAGE)
