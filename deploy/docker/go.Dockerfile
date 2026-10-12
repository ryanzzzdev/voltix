# Builds the Go binaries (controller + upstream mock) into one image.
# Services select their entrypoint via compose `command:`.
FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/voltix ./cmd/voltix \
 && CGO_ENABLED=0 go build -trimpath -o /out/upstream-mock ./cmd/upstream-mock

FROM debian:bookworm-slim
RUN useradd -r -u 10001 -m voltix
COPY --from=build /out/voltix /out/upstream-mock /usr/local/bin/
# Fresh named volumes inherit this directory's ownership.
RUN mkdir -p /state && chown voltix:voltix /state
USER voltix
