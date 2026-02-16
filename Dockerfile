FROM golang:1.26-alpine AS builder

RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

WORKDIR /go/src/github.com/scaleway/scaleway-filestorage-csi

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/

ARG TAG
ARG COMMIT_SHA
ARG BUILD_DATE
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -ldflags "-w -s -X github.com/scaleway/scaleway-filestorage-csi/pkg/driver.driverVersion=${TAG} -X github.com/scaleway/scaleway-filestorage-csi/pkg/driver.buildDate=${BUILD_DATE} -X github.com/scaleway/scaleway-filestorage-csi/pkg/driver.gitCommit=${COMMIT_SHA} " -o scaleway-filestorage-csi ./cmd/scaleway-filestorage-csi
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -ldflags "-w -s -X github.com/scaleway/scaleway-filestorage-csi/pkg/driver.driverVersion=${TAG} -X github.com/scaleway/scaleway-filestorage-csi/pkg/driver.buildDate=${BUILD_DATE} -X github.com/scaleway/scaleway-filestorage-csi/pkg/driver.gitCommit=${COMMIT_SHA} " -o node-discovery ./cmd/node-discovery

FROM alpine:3.23
RUN apk update && apk add --no-cache ca-certificates && update-ca-certificates
WORKDIR /
COPY --from=builder /go/src/github.com/scaleway/scaleway-filestorage-csi/scaleway-filestorage-csi .
COPY --from=builder /go/src/github.com/scaleway/scaleway-filestorage-csi/node-discovery .

ENTRYPOINT ["/scaleway-filestorage-csi"]
