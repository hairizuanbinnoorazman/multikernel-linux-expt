SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

PROJECT ?= REDACTED_GCP_PROJECT_ID
ZONE ?= asia-southeast1-b
INSTANCE ?= mklinux-lab
MACHINE_TYPE ?= n2-standard-16
IMAGE_PROJECT ?= ubuntu-os-cloud
IMAGE_FAMILY ?= ubuntu-2604-lts-amd64
DISK_SIZE ?= 100GB
REMOTE_LAB ?= multikernel-linux-lab

GCLOUD = gcloud compute
SSH = $(GCLOUD) ssh $(INSTANCE) --project=$(PROJECT) --zone=$(ZONE)

.PHONY: help check-gcloud vm-create vm-describe vm-start vm-stop vm-delete \
	ssh serial snapshot sync provision-kernel reboot verify-host install-kerf \
	smoke-up smoke-status smoke-down collect-logs

help: ## Show available targets.
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "%-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check-gcloud: ## Show the active account and selected project.
	gcloud auth list --filter=status:ACTIVE
	gcloud projects describe $(PROJECT) --format='value(projectId,lifecycleState)'

vm-create: ## Create the GCE laboratory VM (billable).
	$(GCLOUD) instances create $(INSTANCE) \
		--project=$(PROJECT) \
		--zone=$(ZONE) \
		--machine-type=$(MACHINE_TYPE) \
		--image-project=$(IMAGE_PROJECT) \
		--image-family=$(IMAGE_FAMILY) \
		--boot-disk-type=pd-balanced \
		--boot-disk-size=$(DISK_SIZE) \
		--no-shielded-secure-boot \
		--shielded-vtpm \
		--shielded-integrity-monitoring \
		--metadata=serial-port-enable=true

vm-describe: ## Show VM state, machine type, IP, disk, and shielded settings.
	$(GCLOUD) instances describe $(INSTANCE) --project=$(PROJECT) --zone=$(ZONE)

vm-start: ## Start the laboratory VM.
	$(GCLOUD) instances start $(INSTANCE) --project=$(PROJECT) --zone=$(ZONE)

vm-stop: ## Stop the VM to stop vCPU/RAM charges (disk charges remain).
	$(GCLOUD) instances stop $(INSTANCE) --project=$(PROJECT) --zone=$(ZONE)

vm-delete: ## Delete the VM and its auto-delete boot disk; prompts through gcloud.
	$(GCLOUD) instances delete $(INSTANCE) --project=$(PROJECT) --zone=$(ZONE)

ssh: ## Open an interactive SSH session.
	$(SSH)

serial: ## Print serial-port output.
	$(GCLOUD) instances get-serial-port-output $(INSTANCE) \
		--project=$(PROJECT) --zone=$(ZONE) --port=1

snapshot: ## Snapshot the current boot disk as a recovery point.
	$(GCLOUD) snapshots create $(INSTANCE)-manual-$$(date +%Y%m%d-%H%M%S) \
		--project=$(PROJECT) \
		--source-disk=$(INSTANCE) \
		--source-disk-zone=$(ZONE)

sync: ## Copy the verified remote scripts and child init into the VM.
	$(SSH) --command='mkdir -p ~/$(REMOTE_LAB)/guest ~/$(REMOTE_LAB)/scripts'
	$(GCLOUD) scp guest/init $(INSTANCE):~/$(REMOTE_LAB)/guest/ \
		--project=$(PROJECT) --zone=$(ZONE)
	$(GCLOUD) scp scripts/*.sh $(INSTANCE):~/$(REMOTE_LAB)/scripts/ \
		--project=$(PROJECT) --zone=$(ZONE)
	$(SSH) --command='chmod +x ~/$(REMOTE_LAB)/guest/init ~/$(REMOTE_LAB)/scripts/*.sh'

provision-kernel: sync ## Install dependencies, build, and install v7.0-mk2; does not reboot.
	$(SSH) --command='~/$(REMOTE_LAB)/scripts/provision-kernel.sh'

reboot: ## Reboot, then wait until SSH reports the expected custom release.
	-$(SSH) --command='sudo systemctl reboot'
	@for attempt in $$(seq 1 24); do \
		sleep 5; \
		if $(SSH) --command='test "$$(uname -r)" = 7.0.0-mk2-gce-lab' >/dev/null 2>&1; then \
			echo 'custom kernel is reachable'; exit 0; \
		fi; \
	done; \
	echo 'custom kernel did not become reachable; inspect make serial' >&2; exit 1

verify-host: sync ## Verify the custom host kernel and essential GCE functions.
	$(SSH) --command='INSTANCE=$(INSTANCE) ~/$(REMOTE_LAB)/scripts/verify-host.sh'

install-kerf: sync ## Build/install pinned Kerf with the Ubuntu 26.04 Python workaround.
	$(SSH) --command='~/$(REMOTE_LAB)/scripts/install-kerf.sh'

smoke-up: sync ## Reserve a pool and boot two children; leaves both running.
	$(SSH) --command='~/$(REMOTE_LAB)/scripts/smoke-up.sh'

smoke-status: ## Prove host and both child statuses while the smoke test is up.
	$(SSH) --command='uname -r; systemctl is-active google-guest-agent; \
		echo smoke-a=$$(cat /sys/fs/multikernel/instances/smoke-a/status); \
		echo smoke-b=$$(cat /sys/fs/multikernel/instances/smoke-b/status); \
		sudo ~/src/kerf/.venv/bin/kerf show'

smoke-down: sync ## Stop/delete both children and return the whole pool to the host.
	$(SSH) --command='~/$(REMOTE_LAB)/scripts/smoke-down.sh'

collect-logs: ## Save serial output and a live host report under logs/.
	mkdir -p logs
	$(GCLOUD) instances get-serial-port-output $(INSTANCE) \
		--project=$(PROJECT) --zone=$(ZONE) --port=1 >logs/serial.txt
	$(SSH) --command='uname -a; cat /proc/kimage; sudo dmesg; \
		sudo ~/src/kerf/.venv/bin/kerf show' >logs/host.txt
