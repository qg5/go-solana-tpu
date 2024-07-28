package quic

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/quic-go/quic-go"
)

// https://github.com/anza-xyz/agave/blob/70efe819bcf70476926f06c473f29015b3269536/streamer/src/nonblocking/quic.rs#L72
const ALPN_TPU_PROTOCOL_ID = "solana-tpu"

func createTlsConfig() (*tls.Config, error) {
	certPem, privPem := newDummyX509Certificate()
	cert, err := tls.X509KeyPair(certPem, privPem)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{ALPN_TPU_PROTOCOL_ID},
		Certificates:       []tls.Certificate{cert},
		MinVersion:         tls.VersionTLS13,
		ServerName:         "server",
	}, nil
}

// SendTransaction send a serialized transaction to the given address using QUIC
func SendTransaction(ctx context.Context, addr string, serializedTx []byte) error {
	tlsConfig, err := createTlsConfig()
	if err != nil {
		return err
	}

	conn, err := quic.DialAddr(ctx, addr, tlsConfig, &quic.Config{
		MaxIdleTimeout: 30 * time.Second,
	})
	if err != nil {
		return err
	}
	defer conn.CloseWithError(0, "success")

	stream, err := conn.OpenUniStreamSync(ctx)
	if err != nil {
		return err
	}

	if _, err = stream.Write(serializedTx); err != nil {
		return err
	}

	if err := stream.Close(); err != nil {
		return err
	}

	return nil
}
