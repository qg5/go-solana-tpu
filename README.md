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

## Considerations

2. **TPU and Transaction speed**: Using TPU while still mishandling fees will not get your transaction included in the block faster
3. **Signing**: Transactions sent using this package MUST be signed, we don't sign them for you
4. **LiteRPC**: It's not recommended to use this in your programs since it's specifically designed for this package, it collects just the right amount of data that it needs

## Alternatives

- [Typescript TPU Client](https://github.com/lmvdz/tpu-client)
- [Rust TPU Client](https://crates.io/crates/solana-tpu-client)
