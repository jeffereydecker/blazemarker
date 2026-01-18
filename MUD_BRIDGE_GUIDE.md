# MUD Bridge - Standalone Funklord Proxy

The MUD Bridge is a standalone program that connects to funklord.com and proxies all communication through Blazemarker's chat system. This allows you to interact with funklord from anywhere via Blazemarker chat, and keeps your funklord session alive even when you're not actively browsing.

## Architecture

```
You (at work/home) 
    ↕ Blazemarker Chat
    ↕ MUD Bridge (running anywhere)
    ↕ funklord.com
```

## Features

- **Standalone Process**: Runs independently from the Blazemarker web server
- **Session Keep-Alive**: Sends periodic commands to prevent funklord timeout
- **Remote Access**: Control funklord from any device with Blazemarker access
- **Command Proxy**: Commands sent to "funklord" user in Blazemarker chat are forwarded to funklord.com
- **Response Proxy**: All funklord output appears as messages from "funklord" in Blazemarker chat

## Installation

```bash
cd mud_bridge
go build
```

## Usage

### Basic Usage

```bash
./mud_bridge -user jdecker -pass blazemarker_password \
             -mud-user jdecker -mud-pass funklord_password
```

### With Custom Server

```bash
./mud_bridge -url https://blazemarker.com \
             -user jdecker -pass blazemarker_password \
             -mud-user jdecker -mud-pass funklord_password
```

### Disable Keep-Alive

```bash
./mud_bridge -user jdecker -pass blazemarker_password \
             -mud-user jdecker -mud-pass funklord_password \
             -keep-alive 0
```

### Custom Keep-Alive Interval

```bash
./mud_bridge -user jdecker -pass blazemarker_password \
             -mud-user jdecker -mud-pass funklord_password \
             -keep-alive 10  # Every 10 minutes
```

## Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-url` | `http://localhost:3000` | Blazemarker server URL |
| `-user` | (required) | Your Blazemarker username |
| `-pass` | (required) | Your Blazemarker password |
| `-mud-user` | (required) | Your funklord username |
| `-mud-pass` | (required) | Your funklord password |
| `-keep-alive` | `5` | Send keep-alive command every N minutes (0=disabled) |

## How to Use

1. **Start the bridge**:
   ```bash
   ./mud_bridge -user jdecker -pass mypass -mud-user jdecker -mud-pass mudpass
   ```

2. **Open Blazemarker chat** in your browser (anywhere - work, home, mobile)

3. **Chat with "funklord"** user:
   - Send commands: `look`, `say hello`, `north`, etc.
   - Receive responses: All funklord output appears as chat messages

4. **Keep-alive works automatically**: Every 5 minutes (by default), the bridge sends `look` to keep your session active

## Running as a Background Service

### On macOS/Linux

```bash
# Start in background
nohup ./mud_bridge -user jdecker -pass mypass -mud-user jdecker -mud-pass mudpass &

# Check if running
ps aux | grep mud_bridge

# Stop
pkill mud_bridge
```

### Using screen

```bash
# Start in screen session
screen -S mudbridge
./mud_bridge -user jdecker -pass mypass -mud-user jdecker -mud-pass mudpass

# Detach: Ctrl+A, then D
# Reattach: screen -r mudbridge
```

## Running at Different Locations

### At Home (connecting to remote Blazemarker)

```bash
./mud_bridge -url https://blazemarker.com \
             -user jdecker -pass mypass \
             -mud-user jdecker -mud-pass mudpass
```

### At Work (same setup)

```bash
./mud_bridge -url https://blazemarker.com \
             -user jdecker -pass mypass \
             -mud-user jdecker -mud-pass mudpass
```

**Note**: Only run one bridge instance at a time to avoid conflicts!

## Requirements

- Chrome/Chromium installed (for headless browser)
- Network access to both Blazemarker and funklord.com
- Valid credentials for both systems

## Troubleshooting

### "chromium not found"

Install Chrome/Chromium:
- **macOS**: `brew install --cask google-chrome`
- **Linux**: `apt install chromium-browser` or `yum install chromium`
- **OpenBSD**: `pkg_add chromium`

### "Failed to login to funklord.com"

- Check your funklord credentials
- Ensure funklord.com is accessible from your network
- Check if funklord's login process has changed

### "Error fetching messages"

- Verify Blazemarker URL is correct
- Check your Blazemarker credentials
- Ensure Blazemarker server is running and accessible

## Advantages Over Server-Integrated Approach

1. **Portability**: Run anywhere with Chrome installed
2. **Independence**: Doesn't require OpenBSD/server infrastructure
3. **Flexibility**: Easy to start/stop without affecting web server
4. **Multiple Instances**: Could run different sessions (though not recommended)
5. **Debugging**: Easier to test and troubleshoot standalone

## Security Notes

- Credentials are passed as command-line arguments (visible in process list)
- Consider using environment variables for production:
  ```bash
  export BLAZEMARKER_USER=jdecker
  export BLAZEMARKER_PASS=mypass
  # Then modify code to read from env vars
  ```
- Keep-alive prevents timeout but uses bandwidth
- Bridge must stay running for session to remain active
