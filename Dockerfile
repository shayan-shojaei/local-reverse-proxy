# syntax=docker/dockerfile:1.7
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS controller
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/lrp-server ./cmd/lrp-server

FROM alpine:3.22
RUN addgroup -S lrp && adduser -S -G lrp -u 10001 lrp
WORKDIR /app
COPY --from=controller /out/lrp-server /usr/local/bin/lrp-server
COPY --from=web /src/web/dist /app/web
RUN mkdir /data && chown lrp:lrp /data
USER lrp
EXPOSE 7400
ENTRYPOINT ["lrp-server"]
