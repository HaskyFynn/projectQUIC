package main

import (
	// Import necessary libraries
	"crypto/md5"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"

	_ "net/http/pprof" // Imported but not used (commented out)

	"gotest/my_testdata" // my_testdata provides helper functions

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/quic-go/qlog"
)

// Custom type to represent a list of bind addresses
type binds []string

// Implement the String method for binds type to format the list as a comma-separated string
func (b binds) String() string {
	return strings.Join(b, ",")
}

// Implement the Set method for binds type to parse comma-separated bind addresses from the flag value
func (b *binds) Set(v string) error {
	*b = strings.Split(v, ",")
	return nil
}

// Interface to represent objects that can report their size
type Size interface {
	Size() int64
}

// Generate pseudo-random data using the Lehmer random number generator algorithm
func generatePRData(l int) []byte {
	res := make([]byte, l)
	seed := uint64(1)
	for i := 0; i < l; i++ {
		seed = seed * 48271 % 2147483647
		res[i] = byte(seed)
	}
	return res
}

// Function to set up the HTTP request handler based on the provided www directory path
func setupHandler(www string) http.Handler {
	mux := http.NewServeMux()

	// Serve static content from the www directory if provided
	if len(www) > 0 {
		mux.Handle("/", http.FileServer(http.Dir(www)))
	} else {
		// Handle requests on the root path by generating pseudo-random data of requested size
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Printf("%#v\n", r)  // Log the request details (commented out for potential performance reasons)
			const maxSize = 1 << 30 // Maximum allowed size: 1 GB
			num, err := strconv.ParseInt(strings.ReplaceAll(r.RequestURI, "/", ""), 10, 64)
			if err != nil || num <= 0 || num > maxSize {
				w.WriteHeader(400) // Send Bad Request (400) for invalid requests
				return
			}
			w.Write(generatePRData(int(num)))
		})
	}

	// Define handlers for various demo endpoints
	mux.HandleFunc("/demo/tile", func(w http.ResponseWriter, r *http.Request) {
		// Serve a small pre-defined PNG image as a tile
		w.Write([]byte{
			0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
			0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x28, 0x00, 0x00, 0x00, 0x28,
			0x01, 0x03, 0x00, 0x00, 0x00, 0xb6, 0x30, 0x2a, 0x2e, 0x00, 0x00, 0x00,
			0x03, 0x50, 0x4c, 0x54, 0x45, 0x5a, 0xc3, 0x5a, 0xad, 0x38, 0xaa, 0xdb,
			0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41, 0x54, 0x78, 0x01, 0x63, 0x18,
			0x61, 0x00, 0x00, 0x00, 0xf0, 0x00, 0x01, 0xe2, 0xb8, 0x75, 0x22, 0x00,
			0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
		})
	})

	mux.HandleFunc("/demo/tiles", func(w http.ResponseWriter, r *http.Request) {
		// Generate an HTML page with 200 image tiles referencing the /demo/tile endpoint
		io.WriteString(w, "<html><head><style>img{width:40px;height:40px;}</style></head><body>")
		for i := 0; i < 200; i++ {
			fmt.Fprintf(w, `<img src="/demo/tile?cachebust=%d">`, i)
		}
		io.WriteString(w, "</body></html>")
	})

	mux.HandleFunc("/demo/echo", func(w http.ResponseWriter, r *http.Request) {
		// Echo back the request body content
		body, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("error reading body while handling /echo: %s\n", err.Error())
		}
		w.Write(body)
	})

	// Handler for uploading files and returning the MD5 hash
	mux.HandleFunc("/demo/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// Parse multipart form data with a maximum size of 1 GB
			err := r.ParseMultipartForm(1 << 30)
			if err == nil {
				var file multipart.File
				file, _, err = r.FormFile("uploadfile")
				if err == nil {
					var size int64
					// Check if the uploaded file implements the Size interface
					if sizeInterface, ok := file.(Size); ok {
						size = sizeInterface.Size()
						b := make([]byte, size)
						// Read the entire file content into a byte buffer
						file.Read(b)
						md5 := md5.Sum(b)
						// Write the MD5 hash of the uploaded file in hexadecimal format
						fmt.Fprintf(w, "%x", md5)
						return
					}
					err = errors.New("couldn't get uploaded file size")
				}
			}
			// Log errors during upload processing
			log.Printf("Error receiving upload: %#v", err)
		}
		// Provide an HTML form for uploading files if the request is not a POST
		io.WriteString(w, `<html><body><form action="/demo/upload" method="post" enctype="multipart/form-data">
                <input type="file" name="uploadfile"><br>
                <input type="submit">
            </form></body></html>`)
	})

	return mux
}

func main() {
	// Commented out profiling and blocking profile rate settings (unrelated to core functionality)
	// defer profile.Start().Stop()
	// go func() {
	//     log.Println(http.ListenAndServe("localhost:6060", nil))
	// }()
	// runtime.SetBlockProfileRate(1)

	// Define command-line flags
	bs := binds{}
	flag.Var(&bs, "bind", "comma-seperated addresses, example: 10.10.10.254:6121")
	www := flag.String("www", "", "path to directory containing static content")
	tcp := flag.Bool("tcp", false, "listen on TCP (in addition to QUIC)")
	key := flag.String("key", "", "path to TLS key file (required with -cert)")
	cert := flag.String("cert", "", "path to TLS certificate file (required with -key)")
	flag.Parse()

	// Set default bind address if none provided
	if len(bs) == 0 {
		bs = binds{"localhost:6121"}
	}

	// Set up the HTTP request handler
	handler := setupHandler(*www)

	// Wait group for goroutines handling server listeners
	var wg sync.WaitGroup
	wg.Add(len(bs))

	// Determine TLS certificate and key file paths
	var certFile, keyFile string
	if *key != "" && *cert != "" {
		keyFile = *key
		certFile = *cert
	} else {
		certFile, keyFile = my_testdata.GetCertificatePaths() // This function provides default paths
	}

	// Start server listeners on each provided bind address
	for _, b := range bs {
		fmt.Println("listening on", b)
		bCap := b // Capture loop variable for goroutine
		go func() {
			var err error
			if *tcp {
				// Listen on TCP using HTTP/3
				err = http3.ListenAndServe(bCap, certFile, keyFile, handler)
			} else {
				// Listen on QUIC using HTTP/3 with TLS configuration
				server := http3.Server{
					Handler: handler,
					Addr:    bCap,
					QUICConfig: &quic.Config{
						Tracer: qlog.DefaultTracer,
					},
				}
				err = server.ListenAndServeTLS(certFile, keyFile)
			}
			if err != nil {
				fmt.Println(err)
			}
			wg.Done()
		}()
	}

	// Wait for all server listeners to finish
	wg.Wait()
}
