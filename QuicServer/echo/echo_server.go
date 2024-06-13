package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"

	"github.com/quic-go/quic-go"
)

const (
	addr    = "localhost:4242" // Address to listen on (server) and connect to (client)
	message = "afa papa!"      // Message to send and echo
)

// We start a server echoing data on the first stream the client opens,
// then connect with a client, send the message, and wait for its receipt.
func main() {
	// Start the echo server in a goroutine
	go func() {
		if err := echoServer(); err != nil {
			log.Fatal(err)
		}
	}()

	// Run the client functionality
	if err := clientMain(); err != nil {
		panic(err)
	}
}

// Start a server that echos all data on the first stream opened by the client
func echoServer() error {
	// Create a QUIC listener with TLS configuration
	listener, err := quic.ListenAddr(addr, generateTLSConfig(), nil)
	if err != nil {
		return err
	}
	defer listener.Close()

	// Accept a QUIC connection from the client
	conn, err := listener.Accept(context.Background())
	if err != nil {
		return err
	}

	// Accept the first stream opened by the client
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		panic(err)
	}
	defer stream.Close()

	// Echo all data received on the stream through the loggingWriter
	_, err = io.Copy(loggingWriter{stream}, stream)
	return err
}

// Run the client that sends a message and waits for the echo
func clientMain() error {
	// Configure TLS connection with InsecureSkipVerify for simplicity (not recommended in production)
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"}, // Advertise the application protocol
	}

	// Dial the server address with the TLS configuration
	conn, err := quic.DialAddr(context.Background(), addr, tlsConf, nil)
	if err != nil {
		return err
	}
	defer conn.CloseWithError(0, "")

	// Open a stream to send and receive data
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return err
	}
	defer stream.Close()

	// Print a message indicating data is being sent
	fmt.Printf("Client: Sending '%s'\n", message)

	// Write the message to the stream
	_, err = stream.Write([]byte(message))
	if err != nil {
		return err
	}
	// Allocate a buffer to store the echoed message
	buf := make([]byte, len(message))

	// Read the entire echoed message from the stream
	_, err = io.ReadFull(stream, buf)
	if err != nil {
		return err
	}

	// Print a message indicating the echoed message has been received
	fmt.Printf("Client: Got '%s'\n", buf)

	return nil
}

// A wrapper for io.Writer that also logs the message received by the server
type loggingWriter struct {
	io.Writer
}

func (w loggingWriter) Write(b []byte) (int, error) {
	// Log the received message
	fmt.Printf("Server: Got '%s'\n", string(b))
	// Write the message to the underlying writer (ie the echo stream)
	return w.Writer.Write(b)
}

// Generate a self-signed TLS configuration for demonstration purposes
func generateTLSConfig() *tls.Config {
	// Generate a RSA key pair
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}

	// Create a basic certificate template
	template := x509.Certificate{SerialNumber: big.NewInt(1)}

	// Generate the DER-encoded certificate data
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"quic-echo-example"},
	}
}
