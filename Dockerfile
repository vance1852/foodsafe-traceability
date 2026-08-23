FROM golang:1.22.5-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/foodsafe-traceability ./cmd/server \
    && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/foodsafe-traceability /app/foodsafe-traceability
COPY --chown=nonroot:nonroot --from=build /out/data /data
VOLUME ["/data"]
ENV FOODSAFE_HTTP_ADDR=:8080
ENV FOODSAFE_DATABASE_PATH=/data/foodsafe.db
EXPOSE 8080
ENTRYPOINT ["/app/foodsafe-traceability"]
