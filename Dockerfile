FROM scratch AS base
ENTRYPOINT ["/gateway-lens"]

FROM base AS local
COPY bin/gateway-lens /

FROM base AS goreleaser
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/gateway-lens /
