FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -o /bin/educraft-api ./cmd/api
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /bin/educraft-api /app/api
COPY --from=build /src/migrations /app/migrations
EXPOSE 8080
ENTRYPOINT ["/app/api"]
