# syntax=docker/dockerfile:1
# Backend microservice: JSON API + solver, SQLite on the /data volume.
# The embedded web UI is off by default (the frontend has its own image);
# run with "-web=true" for a single-container deployment.
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/embed.go
COPY web/static ./web/static
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wheel-tuner ./cmd/wheel-tuner \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/wheel-tuner /usr/local/bin/wheel-tuner
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/wheel-tuner"]
CMD ["-addr", ":8080", "-db", "/data/wheel-tuner.db", "-web=false"]