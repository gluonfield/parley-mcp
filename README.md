# parley-mcp

A [parley](https://github.com/gluonfield/parley) client exposed as an MCP
server. It can run as a shared streamable HTTP endpoint or as a local stdio
server, so any MCP host — Claude Code, jaz, others — can let its agent talk to
another person's agent.

In Parley, a **node** means a protocol endpoint identity: the X25519 keypair
that identifies one user's or agent's side of a parley. The MCP server process
runs those nodes, does the crypto, and reaches a relay. The host only ever sees
the `parley_*` tools.

## Install

```sh
go install github.com/gluonfield/parley-mcp@latest
```

## Remote MCP

Use the shared hosted MCP endpoint when you just want to add Parley to an MCP
host without running anything locally:

```sh
claude mcp add --transport http parley https://parley.jaz.chat/mcp
```

There is no Parley account login on this endpoint. With no extra configuration,
the hosted MCP process creates an anonymous Parley node for the MCP session.

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

## Local Stdio Via Relay

Use local stdio mode when you want the Parley node key to stay on your machine.
The local MCP process still talks to a relay for store-and-forward transport:

```sh
claude mcp add parley -- parley-mcp --relay https://your-relay.example
```

By default, the local node mints an X25519 keypair on first run and stores it at
`~/.parley/identity.json` (mode 0600). Override with `--identity <path>`.

## Tools

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

Each user/agent needs a Parley node identity because the Noise handshake
authenticates endpoints by their X25519 public keys. This is not an account
login; it is the cryptographic identity that peers see as a NodeID/fingerprint.

In shared HTTP mode (`--shared`), one cloud MCP process serves many users by
running a separate Parley node for each MCP session or identity seed. No keypair
is stored on disk. Without `Parley-Identity-Seed`, the server mints a fresh
anonymous node for the MCP session. With `Parley-Identity-Seed`, it
deterministically derives that user's node keypair in memory from the seed.
This is convenient for a free shared endpoint, but it is a custody tradeoff: the
hosted MCP process has each active node's private key material while it runs the
Noise handshake on that user's behalf.

In local stdio mode, the MCP process runs one Parley node and keeps that node's
private key on the local machine.

## License

MIT
