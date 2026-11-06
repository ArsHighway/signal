FROM node:22-bookworm-slim AS web
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY tsconfig.json vite.config.ts ./
COPY web ./web
RUN npm run build

FROM golang:1.23-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY db ./db
RUN CGO_ENABLED=0 go build -trimpath -o /signalcare ./cmd/server

FROM debian:bookworm-slim
RUN useradd --system --uid 10001 signal
WORKDIR /app
COPY --from=web /src/dist ./dist
COPY --from=backend /signalcare ./signalcare
USER signal
ENV APP_ADDR=0.0.0.0:8087
EXPOSE 8087
CMD ["./signalcare"]
