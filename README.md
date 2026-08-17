# esqlorer

A Go-based TUI for querying Elasticsearch using ES|QL. It mimics the Kibana
Discover experience, providing a lightweight, terminal-native way to explore
data with support for multiple environments.

## Requirements

- Go 1.26+

## Build

```sh
go build -o esqlorer ./cmd
```

## Run

```sh
./esqlorer
```

Or run directly without building a binary:

```sh
go run ./cmd
```

## Configuration

Add a server using username/password (basic auth) credentials:

```sh
./esqlorer auth add --name local --url http://localhost:9200 --auth-method basic --username elastic --password changeme
```

Or run `./esqlorer auth add` without flags to add a server interactively.

Other server management commands:

```sh
./esqlorer auth list           # list configured servers
./esqlorer auth switch local   # switch the active server
./esqlorer auth remove local   # remove a server
```

Servers are stored in `~/.config/esqlorer/config.yaml` by default (override with `--config`).
