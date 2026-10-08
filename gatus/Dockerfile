# Build context: repository root. Gatus with the greengrade configuration, running as non-root.
FROM busybox:1.37 AS prep
RUN mkdir -p /out/data && chown 65534:65534 /out/data

FROM ghcr.io/twin/gatus:v5.37.0
COPY --from=prep --chown=65534:65534 /out/data /data
COPY gatus/config.yaml /config/config.yaml
USER 65534:65534
