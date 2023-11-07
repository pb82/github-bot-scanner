FROM registry.access.redhat.com/ubi9/go-toolset:1.19 as builder

USER root

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY main.go main.go

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o scanner main.go

FROM python:3.11.6

USER root
RUN useradd -ms /bin/bash scanner

USER scanner
WORKDIR /home/scanner
ENV PATH="${PATH}:/home/scanner/venv/bin"
ENV HOME=/home/scanner
RUN python3 -m venv /home/scanner/venv
RUN . /home/scanner/venv/bin/activate && pip install ansible-lint
COPY --from=builder /workspace/scanner $APP_ROOT
COPY ansible-lint-config.yml $APP_ROOT

USER root
RUN chmod -R 777 /home/scanner

USER scanner
ENTRYPOINT ["/bin/sh", "-c", "/home/scanner/scanner 2> /dev/termination-log"]
