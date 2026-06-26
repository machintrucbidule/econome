# syntax=docker/dockerfile:1
# Multi-stage, multi-arch build (technical/07 §1, T8). The pure-Go SQLite driver
# (I-001) lets us build a fully static binary with CGO_ENABLED=0 and
# cross-compile cleanly to linux/amd64 + linux/arm64 from one builder.
ARG GO_VERSION=1.26

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

# Dependency layer (cached).
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/econome ./cmd/econome
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/econome-admin ./cmd/econome-admin

# Minimal final image: distroless static (CA certs + nonroot user, no shell).
FROM gcr.io/distroless/static-debian12:nonroot AS final
COPY --from=build /out/econome /econome
COPY --from=build /out/econome-admin /econome-admin

ENV ECONOME_DATA_DIR=/data
EXPOSE 8765
VOLUME ["/data"]
USER nonroot:nonroot
ENTRYPOINT ["/econome"]
