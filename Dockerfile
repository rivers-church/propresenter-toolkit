# ProPresenter Toolkit - web app container.
#   docker compose up -d --build     then open http://<host>:5000
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/pptoolkit ./cmd/pptoolkit \
 && mkdir -p /out/data/styles

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/pptoolkit /pptoolkit
# Style profiles live on a volume so they survive rebuilds. On first start
# with an empty volume the app copies in its built-in example styles.
COPY --from=build --chown=nonroot:nonroot /out/data /data
ENV PPT_STYLES_DIR=/data/styles \
    PPT_ADDR=:5000
VOLUME ["/data/styles"]
EXPOSE 5000
ENTRYPOINT ["/pptoolkit", "serve"]
