# PaperSage server — API-only static binary.
#
# Build:  docker build -t papersage -f server/Dockerfile .
# Run:    docker run -p 5478:5478 papersage

FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY server/ .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/papersage ./cmd/papersage

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/papersage /papersage
ENV PORT=5478
EXPOSE 5478
USER nonroot:nonroot
ENTRYPOINT ["/papersage"]
