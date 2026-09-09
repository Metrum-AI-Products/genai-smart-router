# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0
FROM python:3.13-slim-trixie@sha256:9d2e5553305c7c7b0097999bb17187c69b921ccd6bc9d40e4bb5ebe652c00285 AS build
RUN apt-get update && apt-get install -y --no-install-recommends libgomp1 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY pyproject.toml uv.lock README.md ./
COPY lrp ./lrp
RUN pip install --no-cache-dir uv==0.9.9 && uv export --locked --no-dev --no-emit-project -o /requirements.txt && pip install --no-cache-dir --require-hashes --target /opt/lrp -r /requirements.txt && pip install --no-cache-dir --no-deps --target /opt/lrp .
RUN mkdir /runtime-libs && cp /usr/lib/*/libgomp.so.1 /runtime-libs/

FROM gcr.io/distroless/python3-debian13:nonroot@sha256:f3d5ddc6c64a019fe520e7f005f2880be21e6afc461b10a3c15ef2e4edc71e33
COPY --from=build /opt/lrp /opt/lrp
COPY --from=build /runtime-libs /opt/lib
ENV PYTHONPATH=/opt/lrp LD_LIBRARY_PATH=/opt/lib PYTHONDONTWRITEBYTECODE=1
USER 65532:65532
WORKDIR /bundle
ENTRYPOINT ["/usr/bin/python3", "-m", "lrp.cli"]
CMD ["serve", "--bundle", "/bundle", "--config", "/config/lrp.yaml"]
