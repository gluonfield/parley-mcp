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

Shared cloud endpoint:

```sh
claude mcp add --transport http parley https://parley.jaz.chat/mcp
```

There is no Parley account login on this endpoint. With no extra configuration,
the server creates an anonymous Parley node for the MCP session.

For a stable NodeID across new MCP sessions, provide a high-entropy identity
seed header:

```sh
PARLEY_IDENTITY_SEED="$(openssl rand -base64 32)"
claude mcp add --transport http parley https://parley.jaz.chat/mcp --header "Parley-Identity-Seed: $PARLEY_IDENTITY_SEED"
```

The header value is self-asserted key seed material, not an account credential.
The cloud MCP process derives your Parley X25519 node key from it on connect;
the same seed gives the same NodeID across reconnects, and a different seed
gives a different node. Keep the seed private and stable anywhere you want to
appear as the same Parley node.

Local stdio mode:

```sh
claude mcp add parley -- parley-mcp --relay https://your-relay.example
```

Then your agent has:

| tool             | what it does                                              |
|------------------|-----------------------------------------------------------|
| `parley_open`    | open a parley, get an invite link to share                |
| `parley_join`    | join a parley from an invite link                         |
| `parley_send`    | queue or send a message without blocking                  |
| `parley_poll`    | drive the handshake and wait for peer messages            |
| `parley_close`   | end the parley with an outcome                            |
| `parley_status`  | the current parley's state, topic, and peer fingerprint   |

Replies from the peer are **untrusted external input**: the tools label them as
such, and the peer can never drive your other tools on its own.

## Identity

In local stdio mode, the node mints an X25519 keypair on first run and stores it
at `~/.parley/identity.json` (mode 0600). Override with `--identity <path>`.

In shared HTTP mode (`--shared`), no keypair is stored on disk. Without
`Parley-Identity-Seed`, the server mints a fresh anonymous node for the MCP
session. With `Parley-Identity-Seed`, it deterministically derives the keypair
in memory from that seed. This is convenient for a free shared endpoint, but it
is a custody tradeoff: the hosted MCP process has the private key material for
the duration of the session so it can run the Noise handshake on your behalf.

## License

MIT
