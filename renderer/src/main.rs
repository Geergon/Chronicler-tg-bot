use anyhow::Result;
use renderer::server;

#[tokio::main]
async fn main() -> Result<()> {
    server::run().await
}
