FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build

WORKDIR /src
RUN apk add --no-cache nodejs npm
ARG TARGETOS
ARG TARGETARCH
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN cd docs-site && npm ci && npm run build
RUN find internal/router/docsdist -mindepth 1 ! -name .keep -exec rm -rf {} + && cp -R docs-site/build/. internal/router/docsdist/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/router ./cmd/router
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/router-token-gen ./cmd/router-token-gen
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/router-usage-report ./cmd/router-usage-report

FROM --platform=$BUILDPLATFORM alpine:3.22 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
WORKDIR /app
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/router /app/bin/router
COPY --from=build /out/router-token-gen /app/bin/router-token-gen
COPY --from=build /out/router-usage-report /app/bin/router-usage-report

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/bin/router"]
CMD ["--config", "/app/config/config.yaml"]
