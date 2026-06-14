FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

WORKDIR /src
ARG TARGETOS
ARG TARGETARCH
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/router ./cmd/router
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/router-token-gen ./cmd/router-token-gen

FROM --platform=$BUILDPLATFORM alpine:3.22 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
WORKDIR /app
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/router /app/bin/router
COPY --from=build /out/router-token-gen /app/bin/router-token-gen

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/bin/router"]
CMD ["--config", "/app/config/config.yaml"]
