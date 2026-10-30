FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/chatgpt-api .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
COPY --from=build /out/chatgpt-api /usr/local/bin/chatgpt-api
COPY --from=build /src/config.json /app/config.json
WORKDIR /app
USER app
ENV PORT=8082
EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/chatgpt-api"]
