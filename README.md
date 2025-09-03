<!--
SPDX-FileCopyrightText: 2019, 2020, 2021, 2022 Alvar Penning
SPDX-FileCopyrightText: 2020 Jonas Höchst
SPDX-FileCopyrightText: 2020 Matthias Axel Kröll
SPDX-FileCopyrightText: 2022, 2024, 2025 Markus Sommer

SPDX-License-Identifier: GPL-3.0-or-later
-->

# dtn7-go
[![PkgGoDev](https://pkg.go.dev/badge/github.com/dtn7/dtn7-go)](https://pkg.go.dev/github.com/dtn7/dtn7-go)

Delay-Tolerant Networking software suite and library based on the Bundle Protocol Version 7 ([RFC 9171](https://datatracker.ietf.org/doc/html/rfc9171)).

### Convergence Layer
A *convergence layer* in bundle protocol parlance is the abstraction for peer-to-peer communication.
We have implemented the following protocols:

- Minimal TCP Convergence-Layer Protocol (`mtcp`) ([draft-ietf-dtn-mtcpcl-01](https://tools.ietf.org/html/draft-ietf-dtn-mtcpcl-01)) (RFC draft expired)
- QUIC Convergence Layer (QUICL) (Custom, not (yet) standardised)

## Software
### Installation

Install the [Go programming language](https://go.dev/), version 1.24 or later.

```bash
git clone https://github.com/dtn7/dtn7-go.git
cd dtn7-go

go build ./cmd/dtnd
go build ./cmd/dtnclient
```

### dtnd
`dtnd` is a delay-tolerant networking daemon.
It acts as a node in the network and can transmit, receive and forward bundles to other nodes.
A node's neighbours may be specified in the configuration or detected within the local network through a peer discovery.
Bundles might be sent and received through a REST-like web interface.
The features and configuration are described inside the provided example [`configuration.toml`](https://github.com/dtn7/dtn7-go/blob/master/cmd/dtnd/configuration.toml).

#### Application Agents
We provide different interfaces to allow communication from external programs with `dtnd`.
A REST API and an API based on UNIX domain sockets.

The REST API allows a client to register itself with an address, receive bundles and create/dispatch new ones simply by POSTing JSON objects to `dtnd`'s RESTful HTTP server.
The endpoints and structure of the JSON objects are described in the [documentation](https://pkg.go.dev/github.com/dtn7/dtn7-go) for the `github.com/dtn7/dtn7-go/agent.RestAgent` type.

The UNIX API is meant to be somewhat simpler and faster than the HTTP-based alternatives.
The message formats are described in `pkg/application_agent/unix_agent/messages.go` type.
Messages are to be marshaled using the [msgpack](https://github.com/vmihailenco/msgpack) format.
The communication protocol is very simple - first send the length of the message (encoded as a uint64, big endian), and then send the message.

### dtnclient
`dtnclient` is a cli for interacting with `dtnd` via the UNIX application agent.
For usage see `dtnclient -h`.

## Go Library
Most components of this software are usable as a Go library.
Those libraries are available within the `pkg`-directory.

For example, the `bpv7`-package contains code for bundle modification, serialization and deserialization and would most likely be the most interesting part.

## Contributing
We warmly welcome any contribution.

Please format your code using [Gofmt](https://blog.golang.org/gofmt).
Further inspection of the code via [golangci-lint](https://github.com/golangci/golangci-lint) is highly recommended.

As a development environment, you may, of course, use whatever you personally like best.
However, we have had a good experience with [GoLand](https://www.jetbrains.com/go/), especially because of the size of the project.

Assuming you have a supported version of the [Go programming language](https://go.dev/) installed, just clone the repository and install the dependencies as documented in the _Installation, From Source_ section above.

### OS-specific
#### macOS
Installing Go via [homebrew](https://brew.sh), should solve permission errors while trying to fetch the dependencies.

## License

This project's code is licensed under the [GNU General Public License version 3 (_GPL-3.0-or-later_)](LICENSE).
