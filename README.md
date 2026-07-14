# Chronicler-tg-bot
Telegram bot for creating quotes as stickers with chat messages. Supports text, media, block quotes, emoji, formatting, and saving in sticker packs.

## 💻 Installation and Usage with Docker Compose

### Prerequisites

- Docker / Podman
- Docker Compose / Podman Compose
- Telegram Bot Token (obtain from [BotFather](https://t.me/BotFather))
- Telegram API_HASH and API_ID (<https://my.telegram.org/apps>)

### Setup Steps

#### Running bot using docker or podman

```
docker run -d --env-file /path/to/.env -v /path/to/config:/config:z -v /path/to/db:/db:z --name chronicler-tg-bot --restart unless-stopped geergon/chronicler-tg-bot:latest
```

or

```
podman run -d --env-file /path/to/.env -v /path/to/config:/config:z -v /path/to/db:/db:z --name chronicler-tg-bot --restart unless-stopped geergon/chronicler-tg-bot:latest
```

:z can be omitted if SELinux is not installed on the system.

#### ... via Docker Compose

1. **Create a `compose.yaml` file (or download from the repository)**  in the project root with the following content:

```yaml
version: "3.8"

services:
  chronicler-tg-bot:
    image: geergon/chronicler-tg-bot:latest
    container_name: chronicler-tg-bot
    restart: unless-stopped
    env_file:
      - .env
    environment:
      APP_ID: "${APP_ID:?Error: APP_ID not set in .env}"
      API_HASH: "${API_HASH:?Error: API_HASH not set in .env}"
      BOT_TOKEN: "${BOT_TOKEN:?Error: BOT_TOKEN not set in .env}"
    volumes:
      - ./config:/config:z
      - ./db:/db:z
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"

```

You must have an .env file in the directory with compose.yaml
Environment Variables

    APP_ID: Telegram API ID (required).
    API_HASH: Telegram API hash (required).
    BOT_TOKEN: Telegram bot token from BotFather (required).

Volumes

    ./config:/config: Directory for the config.yaml file and other configuration data.
    ./db:/db: Directory for storing bot session data (e.g., Telegram session files).

2. **Run the bot using Docker/Podman Compose**:

```bash
    docker-compose up -d
```

```bash
    podman-compose up -d
```

### Stopping the Bot

To stop the bot:

```bash
docker-compose down

```


### Usage

- `/c` — create a quote sticker from a message (in response to a message)
- `/c N` — create a sticker with N messages in a row (maximum 6)
- `/cr` — create a sticker with reply from a message (also support N for create stikcer with multiple messages)
- `/cs` — save a sticker from a message to the chat sticker pack
- `/crand` — send a random saved quote from chat
- `/crsave <url>` — import an existing sticker pack into the quote database

