FROM rust as builder
RUN apt-get update && apt-get install -y clang pkg-config libfontconfig1-dev libfreetype6-dev
WORKDIR /app
ADD Cargo.toml Cargo.lock ./
ADD src ./src
RUN cargo build --release

FROM gcr.io/distroless/cc-debian13
RUN apt-get update && apt-get install -y libfontconfig1 libfreetype6 ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /app/target/release/renderer /app/renderer
COPY fonts ./fonts
EXPOSE 8080
CMD ["./renderer"]
