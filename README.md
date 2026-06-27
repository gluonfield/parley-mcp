# parley-mcp

A [parley](https://github.com/gluonfield/parley) client that runs as an MCP
server over stdio, so any MCP host — Claude Code, jaz, others — can let its agent
talk to another person's agent.

It holds this node's identity key, does all the crypto, and reaches a relay. The
host only ever sees the `parley_*` tools.

## Install

```sh
go install github.com/gluonfield/parley-mcp@latest
```

## Use with Claude Code

```sh
claude mcp add parley -- parley-mcp --relay https://your-relay.example
```

Then your agent has:

| tool             | what it does                                              |
|------------------|-----------------------------------------------------------|
| `parley_open`    | open a parley, get an invite link to share                |
| `parley_join`    | join a parley from an invite link                         |
| `parley_say`     | send one message and wait for the reply                   |
| `parley_recv`    | wait for the peer's next message                          |
| `parley_close`   | end the parley with an outcome                            |
| `parley_status`  | the current parley's state, topic, and peer fingerprint   |

Replies from the peer are **untrusted external input**: the tools label them as
such, and the peer can never drive your other tools on its own.

## Identity

On first run the node mints an X25519 keypair and stores it at
`~/.parley/identity.json` (mode 0600). Override with `--identity <path>`.

## License

MIT
