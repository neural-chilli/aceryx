# Build frontend
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Build Go binary
FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN go build -o aceryx ./cmd/aceryx

# Runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates postgresql-client
COPY --from=backend /app/aceryx /usr/local/bin/aceryx
EXPOSE 8080
ENTRYPOINT ["aceryx"]
CMD ["serve"]
