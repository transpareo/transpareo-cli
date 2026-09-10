# The signing endpoint of a workspace that brings its own keys.
# The binary is the one the release built; goreleaser puts it in
# the build context under its platform directory.
#
#   docker run --rm -p 8443:8443 \
#     -v /etc/transpareo/signer:/keys:ro \
#     -e TRANSPAREO_PLATFORM_KEY=https://acme.transpareo.com/.well-known/transpareo-signing-key.pem \
#     -e TRANSPAREO_SIGNER_HOST=signer.example.com \
#     ghcr.io/transpareo/transpareo-signer:latest
#
# The base image carries the certificate authorities the platform
# key is fetched over and a non-root account to run as; it holds
# no shell, so nothing but the endpoint runs in the container.
FROM gcr.io/distroless/static-debian12:nonroot

ARG TARGETPLATFORM

COPY $TARGETPLATFORM/transpareo /usr/bin/transpareo

ENV TRANSPAREO_CONFIG_DIR=/tmp/transpareo \
    TRANSPAREO_SIGNER_DIR=/keys \
    TRANSPAREO_SIGNER_LISTEN=0.0.0.0:8443

EXPOSE 8443

ENTRYPOINT ["/usr/bin/transpareo", "signer", "serve"]
