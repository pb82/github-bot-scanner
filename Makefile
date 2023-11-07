.PHONY: build
build:
	docker build -t quay.io/pb82/content-scanner:latest .

run:
	docker run --rm quay.io/pb82/content-scanner:latest