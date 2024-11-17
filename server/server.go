package server

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/tomor/netsimulate/config"
)

func newHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		queryNum := r.FormValue("req")
		fmt.Printf("server: handling request %s num %s\n", r.Method, queryNum)

		// Handle server sleep based on the request number
		if cfg.ServerSleepOnSecond && queryNum == "2" {
			fmt.Printf("server: sleeping %.f sec\n", cfg.ServerSleepOnSecondDuration.Seconds())
			time.Sleep(cfg.ServerSleepOnSecondDuration)
		}

		if cfg.ServerSleepBeforeResponse != 0 {
			fmt.Printf("server: sleeping %.1f sec\n", cfg.ServerSleepBeforeResponse.Seconds())
			time.Sleep(cfg.ServerSleepBeforeResponse)
		}

		// Handle different request methods
		switch r.Method {
		case http.MethodGet:
			fmt.Fprintf(w, "GET request handled with query number: %s", queryNum)

		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Error reading request body", http.StatusInternalServerError)
				return
			}
			defer r.Body.Close()
			fmt.Fprintf(w, "POST request handled, Data received: %s\n", string(body))

		default:
			http.Error(w, "Unsupported request method", http.StatusMethodNotAllowed)
		}
	}
}

func startHTTPServer(cfg *config.Config) {
	http.HandleFunc("/", newHandler(cfg)) // Endpoint to handle requests

	// Start the server and listen on port 8080
	server := &http.Server{
		Addr: cfg.ServerAddress,

		// Set idle timeout to simulate server closing connection after being idle
		IdleTimeout: cfg.ServerIdleTimeout,
	}

	fmt.Println("Starting server on " + cfg.ServerAddress)

	var err error
	if cfg.UseTLS {
		err = server.ListenAndServeTLS("server.crt", "server.key")
	} else {
		err = server.ListenAndServe()
	}
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}

var connectionCount int32

func handleConnection(conn net.Conn, cfg *config.Config) {
	// Increment connection counter atomically
	currentConnection := atomic.AddInt32(&connectionCount, 1)

	// Success on 1st, 3rd, ... (allows simulating retry by round tripper)
	if cfg.ServerSuccessResponseOnFirst && currentConnection%2 == 1 {
		// Send HTTP 200 OK response
		sendHTTPResponse(conn, cfg)
		return
	}

	switch cfg.ServerType {
	case config.ServerTypeMultiResponse:
		sendMulti(conn, cfg)
		return
	case config.ServerTypeRST:
		sendRST(conn, cfg)
		return
	default:
		panic("unknown server type")
	}
}

func sendHTTPResponse(conn net.Conn, cfg *config.Config) {
	defer conn.Close()

	if cfg.ServerSleepBeforeResponse != 0 {
		fmt.Printf("server: sleeping %d sec\n", int(cfg.ServerSleepBeforeResponse.Seconds()))
		time.Sleep(cfg.ServerSleepBeforeResponse)
	}

	// as we are not really responding to a real HTTP request, just wait a bit before sending HTTP response
	// the client should send HTTP request
	time.Sleep(100 * time.Millisecond)

	// Write an HTTP 200 OK response
	response := "HTTP/1.1 200 OK\r\n" +
		"Content-Length: 2\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"OK"

	_, err := conn.Write([]byte(response))
	if err != nil {
		fmt.Println("Error writing response:", err)
		return
	}

	fmt.Println("server: Sent HTTP 200 OK response to", conn.RemoteAddr())
}

// Send one answer, then wait and then close the connection
// the idea is that the client sends another HTTP request over this connection and then receives RST
func sendMulti(conn net.Conn, cfg *config.Config) {
	defer conn.Close()

	// Cheating here: not waiting for HTTP request, just assuming it comes and sending HTTP response after 100ms
	time.Sleep(100 * time.Millisecond)

	// Write HTTP 200 OK response
	response := "HTTP/1.1 200 OK\r\n" +
		"Content-Length: 2\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"OK"

	_, err := conn.Write([]byte(response))
	if err != nil {
		fmt.Println("Error writing response:", err)
		return
	}
	fmt.Println("server: Sent HTTP 200 OK response to", conn.RemoteAddr())

	time.Sleep(cfg.ServerMultiCloseConAfter)
	fmt.Printf("server: closing connection\n")
}

func sendRST(conn net.Conn, cfg *config.Config) {
	defer conn.Close()

	// Get raw file descriptor from the connection
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		fmt.Println("server: Not a TCP connection, closing normally")
		return
	}

	// Get the file descriptor from the TCP connection
	file, err := tcpConn.File()
	if err != nil {
		fmt.Println("server: Error getting file descriptor:", err)
		return
	}
	fd := int(file.Fd())
	defer file.Close()

	// Enable SO_LINGER option with Linger = 0, which causes an RST instead of a FIN
	linger := &syscall.Linger{
		Onoff:  1, // Enable linger option
		Linger: 0, // Set to 0 to send RST
	}

	err = syscall.SetsockoptLinger(fd, syscall.SOL_SOCKET, syscall.SO_LINGER, linger)
	if err != nil {
		fmt.Println("server: Error setting SO_LINGER:", err)
		return
	}

	fmt.Println("server: Abruptly closed connection with RST to", conn.RemoteAddr())
}

func startTPCServer(cfg *config.Config) {
	listener, err := net.Listen("tcp", cfg.ServerAddress)
	if err != nil {
		fmt.Println("Error starting bad server:", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("Starting Bad Server on %s\n", cfg.ServerAddress)
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("server: Error accepting connection:", err)
			continue
		}
		go handleConnection(conn, cfg)
	}
}

func Start(cfg *config.Config) {
	if cfg.ServerType == config.ServerTypeHTTP {
		startHTTPServer(cfg)
	} else {
		startTPCServer(cfg)
	}
}
