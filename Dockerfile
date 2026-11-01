FROM rust:1.85-bookworm AS opaque-builder

WORKDIR /opaque
COPY rust/opaque-ffi/Cargo.toml ./Cargo.toml
COPY rust/opaque-ffi/Cargo.lock ./Cargo.lock
COPY rust/opaque-ffi/build.rs ./build.rs
COPY rust/opaque-ffi/cbindgen.toml ./cbindgen.toml
COPY rust/opaque-ffi/src ./src
RUN cargo build --release --locked

FROM golang:1.27.3-bookworm AS builder

WORKDIR /app
ENV CGO_ENABLED=0

COPY go.mod go.sum ./
RUN go mod download

COPY . .

COPY --from=opaque-builder /opaque/target/release/libarkive_opaque.so /app/libarkive_opaque.so

RUN GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/arkive .

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates libgcc-s1 \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/arkive /app/arkive
COPY --from=builder /app/libarkive_opaque.so /app/libarkive_opaque.so

EXPOSE 8080

CMD ["/app/arkive"]
