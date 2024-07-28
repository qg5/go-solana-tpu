# Solana TPU Client

A simple and efficient TPU ([Transaction Processing Unit](https://docs.solanalabs.com/validator/tpu)) client for Solana, utilizing the QUIC protocol for data transmission

This is designed to send a transaction directly to the current leader(s) instead of using RPC, thereby broadcasting the transaction more quickly

[![Go Reference](https://pkg.go.dev/badge/github.com/qg5/go-solana-tpu.svg)](https://pkg.go.dev/github.com/qg5/go-solana-tpu)

## Lifecycle of a transaction in Solana (RPC and TPU)

![tx lifecycle](/docs/img/tx_lifecycle.png)

## Usage

```
go get -u github.com/qg5/go-solana-tpu/tpu
```

Browse the [examples folder](/examples) to see how you can use this package

## Alternatives

- [Typescript TPU Client](https://github.com/lmvdz/tpu-client)
- [Rust TPU Client](https://crates.io/crates/solana-tpu-client)
