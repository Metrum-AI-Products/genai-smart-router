FROM --platform=$BUILDPLATFORM golang:1.26.8-alpine AS build

WORKDIR /src
RUN apk add --no-cache nodejs npm
COPY docs-site/package.json docs-site/package-lock.json ./docs-site/
RUN npm ci --prefix docs-site --no-audit --no-fund
COPY internal/router/admindist/web/package.json internal/router/admindist/web/package-lock.json ./internal/router/admindist/web/
RUN npm ci --prefix internal/router/admindist/web --no-audit --no-fund
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG GO_BUILD_TAGS=""
RUN DOCS_ROUTER_VERSION=$VERSION DOCS_ROUTER_COMMIT=$COMMIT DOCS_ROUTER_BUILD_DATE=$BUILD_DATE npm run build --prefix docs-site
RUN find internal/router/docsdist -mindepth 1 ! -name .keep -exec rm -rf {} + && cp -R docs-site/build/. internal/router/docsdist/
RUN rm -rf internal/router/admindist/static/assets && npm run build --prefix internal/router/admindist/web
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags "$GO_BUILD_TAGS" -ldflags "-X github.com/metrum-ai/router/internal/buildinfo.Version=$VERSION -X github.com/metrum-ai/router/internal/buildinfo.Commit=$COMMIT -X github.com/metrum-ai/router/internal/buildinfo.BuildDate=$BUILD_DATE" -o /out/router ./cmd/router
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags "$GO_BUILD_TAGS" -ldflags "-X github.com/metrum-ai/router/internal/buildinfo.Version=$VERSION -X github.com/metrum-ai/router/internal/buildinfo.Commit=$COMMIT -X github.com/metrum-ai/router/internal/buildinfo.BuildDate=$BUILD_DATE" -o /out/router-token-gen ./cmd/router-token-gen
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags "$GO_BUILD_TAGS" -ldflags "-X github.com/metrum-ai/router/internal/buildinfo.Version=$VERSION -X github.com/metrum-ai/router/internal/buildinfo.Commit=$COMMIT -X github.com/metrum-ai/router/internal/buildinfo.BuildDate=$BUILD_DATE" -o /out/router-usage-report ./cmd/router-usage-report
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags "$GO_BUILD_TAGS" -ldflags "-X github.com/metrum-ai/router/internal/buildinfo.Version=$VERSION -X github.com/metrum-ai/router/internal/buildinfo.Commit=$COMMIT -X github.com/metrum-ai/router/internal/buildinfo.BuildDate=$BUILD_DATE" -o /out/router-migrate ./cmd/router-migrate
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags "$GO_BUILD_TAGS" -ldflags "-X github.com/metrum-ai/router/internal/buildinfo.Version=$VERSION -X github.com/metrum-ai/router/internal/buildinfo.Commit=$COMMIT -X github.com/metrum-ai/router/internal/buildinfo.BuildDate=$BUILD_DATE" -o /out/metrum-genai-smartrouterctl ./cmd/metrum-genai-smartrouterctl
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags "$GO_BUILD_TAGS" -ldflags "-X github.com/metrum-ai/router/internal/buildinfo.Version=$VERSION -X github.com/metrum-ai/router/internal/buildinfo.Commit=$COMMIT -X github.com/metrum-ai/router/internal/buildinfo.BuildDate=$BUILD_DATE" -o /out/smartrouterctl ./cmd/smartrouterctl

FROM --platform=$BUILDPLATFORM alpine:3.22 AS certs
RUN apk add --no-cache ca-certificates

FROM scratch
LABEL org.opencontainers.image.licenses="Apache-2.0"
WORKDIR /app
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /src/LICENSE /LICENSE
COPY --from=build /src/NOTICE /NOTICE
COPY --from=build /src/THIRD_PARTY_NOTICES.md /THIRD_PARTY_NOTICES.md
COPY --from=build /src/MODEL_LICENSES.md /MODEL_LICENSES.md
COPY --from=build /out/router /app/bin/router
COPY --from=build /out/router-token-gen /app/bin/router-token-gen
COPY --from=build /out/router-usage-report /app/bin/router-usage-report
COPY --from=build /out/router-migrate /app/bin/router-migrate
COPY --from=build /out/metrum-genai-smartrouterctl /app/bin/metrum-genai-smartrouterctl
COPY --from=build /out/smartrouterctl /app/bin/smartrouterctl

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/bin/router"]
CMD ["--config", "/app/config/config.yaml"]
