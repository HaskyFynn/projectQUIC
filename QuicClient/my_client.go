package main

// Import necessary libraries
import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"gotest/my_testdata" // my_testdata provides helper functions

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/quic-go/qlog"
)

func main() {
	// Defining command-line flags
	quiet := flag.Bool("q", false, "don't print the data response body")
	keyLogFile := flag.String("keylog", "", "path to file for TLS key log")
	//insecure := flag.Bool("insecure", false, "skip certificate verification") // Comment out for security reasons
	flag.Parse()
	urls := flag.Args() // Getting URLs from command-line arguments

	// Managing key log file (optional)
	var keyLog io.Writer
	if len(*keyLogFile) > 0 {
		f, err := os.Create(*keyLogFile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		keyLog = f
	}

	// Use system certificate pool and add root CA from my_testdata
	pool, err := x509.SystemCertPool()
	if err != nil {
		log.Fatal(err)
	}
	my_testdata.AddRootCA(pool) // Assuming this function adds a specific root CA

	// Configure TLS client with certificate pool, key log (optional), and QUIC settings
	roundTripper := &http3.RoundTripper{
		TLSClientConfig: &tls.Config{
			RootCAs: pool,
			//InsecureSkipVerify: *insecure, // Commented out for security reasons
			KeyLogWriter: keyLog,
		},
		QUICConfig: &quic.Config{
			Tracer: qlog.DefaultTracer,
		},
	}
	defer roundTripper.Close()

	// Create an HTTP client using the configured round tripper
	hclient := &http.Client{
		Transport: roundTripper,
	}

	// Wait group to synchronize multiple fetches
	var wg sync.WaitGroup
	wg.Add(len(urls))

	// Loop through provided URLs and perform concurrent GET requests
	for _, addr := range urls {
		log.Printf("GET %s", addr)
		go func(addr string) {
			rsp, err := hclient.Get(addr)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("Got response for %s: %#v", addr, rsp)

			body := &bytes.Buffer{}
			_, err = io.Copy(body, rsp.Body)
			if err != nil {
				log.Fatal(err)
			}

			// Print response body size or content depending on the quiet flag
			if *quiet {
				log.Printf("Response Body: %d bytes", body.Len())
			} else {
				log.Printf("Response Body (%d bytes):\n%s", body.Len(), body.Bytes())
			}
			wg.Done()
		}(addr)
	}

	// Wait for all concurrent fetches to complete
	wg.Wait()
}
