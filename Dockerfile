# Stage 1: Build
FROM golang:1.27-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG TARGETOS=linux
ARG TARGETARCH=amd64

# Build gateway
FROM builder AS build-gateway
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
    -o /bin/flexgateway ./cmd/gateway

# Build control plane
FROM builder AS build-controlplane
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /bin/flexcontrolplane ./cmd/controlplane

# Build CLI
FROM builder AS build-fgctl
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /bin/fgctl ./cmd/fgctl

# Runtime: Gateway — uses base (not static) for CA certs needed by HTTPS upstreams
FROM gcr.io/distroless/base-debian12:nonroot AS gateway
COPY --from=build-gateway /bin/flexgateway /flexgateway
USER nonroot:nonroot
EXPOSE 8080 8443
ENTRYPOINT ["/flexgateway"]

# Runtime: Control Plane
FROM gcr.io/distroless/static-debian12:nonroot AS controlplane
COPY --from=build-controlplane /bin/flexcontrolplane /flexcontrolplane
EXPOSE 8081
ENTRYPOINT ["/flexcontrolplane"]

# Runtime: CLI
FROM gcr.io/distroless/static-debian12:nonroot AS fgctl
COPY --from=build-fgctl /bin/fgctl /fgctl
ENTRYPOINT ["/fgctl"]
