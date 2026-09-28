FROM golang:1.25-alpine AS build

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/codeceremony ./cmd/codeceremony

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/codeceremony /codeceremony
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/codeceremony"]
