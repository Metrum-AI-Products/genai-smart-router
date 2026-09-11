# Pinned environment stub for HARBOR-02
# Offline verifier-integrity tests do not build this image.
# Tools are mounted from the task tree at real Harbor runtime.
FROM python:3.11.13-slim-bookworm
WORKDIR /app
COPY workspace/ /app/
CMD ["python3", "-c", "print('harbor-02 env stub')"]
