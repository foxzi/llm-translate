FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/foxzi/llm-translate/internal/cli.Version=${VERSION}" \
    -o /llm-translate ./cmd/llm-translate

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /llm-translate /usr/local/bin/llm-translate
EXPOSE 8080
ENTRYPOINT ["llm-translate"]
CMD ["--api", "-c", "/config.yaml"]
