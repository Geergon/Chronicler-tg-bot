FROM rust as builder
RUN apt-get update && apt-get install -y clang pkg-config libfontconfig1-dev libfreetype6-dev ninja-build
WORKDIR /app
ADD Cargo.toml Cargo.lock ./
ADD src ./src
RUN cargo build --release

FROM debian:trixie-slim
RUN apt-get update && apt-get install -y libfontconfig1 libfreetype6 ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/target/release/renderer /app/renderer
COPY fonts ./fonts
EXPOSE 8080
CMD ["./renderer"]
