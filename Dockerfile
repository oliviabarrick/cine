FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server .

FROM alpine:3.20
WORKDIR /
COPY --from=build /out/server /server
COPY --from=build /src/web /web
EXPOSE 8080
ENTRYPOINT ["/server"]
