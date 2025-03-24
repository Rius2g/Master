REGISTRY=capstonetesting.azurecr.io

.PHONY: all collector experimenter push clean

all: collector experimenter

collector:
	docker build -t $(REGISTRY)/collector-api:latest -f ./docker/Dockerfile.collector .

experimenter:
	docker build -t $(REGISTRY)/experimenter:latest -f ./docker/Dockerfile.experimenter .

push:
	docker push $(REGISTRY)/collector-api:latest
	docker push $(REGISTRY)/experimenter:latest

clean:
	docker rmi $(REGISTRY)/collector-api:latest || true
	docker rmi $(REGISTRY)/experimenter:latest || true

