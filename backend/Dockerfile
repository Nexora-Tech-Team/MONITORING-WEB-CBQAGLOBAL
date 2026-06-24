FROM golang:1.26-alpine AS build

WORKDIR /src

RUN apk add --no-cache git

COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /src/backend
RUN go mod download

COPY backend/ ./
COPY ["Monitoring Pekerjaan.xlsx", "/data/workbook.xlsx"]

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/monitoring-backend .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=build /out/monitoring-backend /app/monitoring-backend
COPY --from=build /data/workbook.xlsx /app/workbook.xlsx

ENV PORT=8090 \
    XLSX_PATH=/app/workbook.xlsx

EXPOSE 8090

CMD ["/app/monitoring-backend"]
