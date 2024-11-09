package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// The simulation can be changed by changing the constants below.
// For many cases you need to comment out one and uncomment another one.
// For starting different type of servers you need to uncomment those lines in main().
// The assumption was that it's one time use script - good enough.
const (
	srvIP   = "127.0.0.1"
	port    = "8080"
	portTLS = "8443"
)

type ServerType int

const (
	ServerTypeHTTP          ServerType = iota
	ServerTypeRST                      // server type: TCP
	ServerTypeMultiResponse            // server type: TCP - server doesn't close the connection after the first response
)

type Config struct {
	ID                           string // Simulation identifier
	Description                  string // Description of the scenario
	ServerAddress                string
	UseHTTPS                     bool          // Enable HTTPS if true
	ServerType                   ServerType    // Type of server simulation (e.g., RST, Abrupt close)
	ClientRequestMethod          string        // HTTP request type (GET, POST, etc.)
	ClientRequestURL             string        // URL to which the client sends the HTTP request
	ServerIdleTimeout            time.Duration // Server idle timeout
	ClientWaitBeforeNextReq      time.Duration // Time client waits before next request
	ClientIdleTimeout            time.Duration // Time for which the TCP connection is kept in the idle pool
	ServerSuccessResponseOnFirst bool          // If the server responds with HTTP 200 OK for the first request (bad server only)
	ServerSleepBeforeResponse    time.Duration // Server sleep duration to simulate delay
	ServerSleepOnSecond          bool          // Simulate sleep only on second request, for: ServerTypeHTTP
	ServerSleepOnSecondDuration  time.Duration // Duration of sleep on second request
	ClientTimeout                time.Duration // Client timeout for each request
	ServerMultiCloseConAfter     time.Duration // Duration after which the connection is closed after the first response for ServerTypeMultiResponse
	ReqCount                     int           // Number of requests client will make
}

func (c Config) print() {
	fmt.Println("Configuration:")
	fmt.Printf("  ID:                           %s\n", c.ID)
	fmt.Printf("  Description:                  %s\n", c.Description)
	fmt.Printf("  Server Address:               %s\n", c.ServerAddress)
	fmt.Printf("  Use HTTPS:                    %t\n", c.UseHTTPS)
	fmt.Printf("  Client Request Type:          %s\n", c.ClientRequestMethod)
	fmt.Printf("  Server Idle Timeout:          %d sec\n", int(c.ServerIdleTimeout.Seconds()))
	fmt.Printf("  Client Idle Timeout:          %d sec\n", int(c.ClientIdleTimeout.Seconds()))
	fmt.Printf("  Client Wait Before Next Req:  %d sec\n", int(c.ClientWaitBeforeNextReq.Seconds()))
	fmt.Printf("  Server Success On First:      %d sec\n", int(c.ServerSleepBeforeResponse.Seconds()))
	fmt.Printf("  Server Sleep Before Response: %d sec\n", int(c.ServerSleepBeforeResponse.Seconds()))
	fmt.Printf("  Server Sleep On Second:       %t\n", c.ServerSleepOnSecond)
	fmt.Printf("  Server Sleep On Second Dur:   %d sec\n", int(c.ServerSleepOnSecondDuration.Seconds()))
	fmt.Printf("  Client Timeout:               %d sec\n", int(c.ClientTimeout.Seconds()))
	fmt.Printf("  Request Count:                %d\n", c.ReqCount)
	fmt.Println()
}

type Simulations []Config

var simulations = Simulations{
	{
		ID:                        "01",
		Description:               "Server HTTP 200 OK response - connection reused from idle pool",
		ClientRequestMethod:       http.MethodGet,
		ServerIdleTimeout:         5 * time.Second,
		ClientIdleTimeout:         90 * time.Second,
		ClientWaitBeforeNextReq:   1 * time.Second,
		ServerSleepBeforeResponse: 0,
		ServerSleepOnSecond:       false,
		ClientTimeout:             10 * time.Second,
		ReqCount:                  3,
		ServerType:                ServerTypeHTTP, // Normal server
	},
	{
		ID:                        "02",
		Description:               "Server HTTP 200 OK response - connection not reused from idle pool - over server idle timeout",
		ClientRequestMethod:       http.MethodGet,
		ServerIdleTimeout:         1 * time.Second,
		ClientIdleTimeout:         90 * time.Second,
		ClientWaitBeforeNextReq:   2 * time.Second,
		ServerSleepBeforeResponse: 0,
		ServerSleepOnSecond:       false,
		ClientTimeout:             10 * time.Second,
		ReqCount:                  3,
		ServerType:                ServerTypeHTTP,
	},
	{
		ID:                        "03",
		Description:               "Server HTTP 200 OK response - connection not reused from idle pool - over client idle timeout",
		ClientRequestMethod:       http.MethodGet,
		ServerIdleTimeout:         5 * time.Second,
		ClientIdleTimeout:         1 * time.Second,
		ClientWaitBeforeNextReq:   2 * time.Second,
		ServerSleepBeforeResponse: 0,
		ServerSleepOnSecond:       false,
		ClientTimeout:             10 * time.Second,
		ReqCount:                  3,
		ServerType:                ServerTypeHTTP,
	},
	{
		ID:                        "04",
		Description:               "Server HTTP 200 OK response - slow response - client timeout - connection not put to idle pool",
		ClientRequestMethod:       http.MethodGet,
		ServerIdleTimeout:         5 * time.Second,
		ClientIdleTimeout:         90 * time.Second,
		ClientWaitBeforeNextReq:   1 * time.Second,
		ServerSleepBeforeResponse: 1100 * time.Millisecond,
		ServerSleepOnSecond:       false,
		ClientTimeout:             1 * time.Second,
		ReqCount:                  3,
		ServerType:                ServerTypeHTTP,
	},
	{
		ID:                          "05",
		Description:                 "Server HTTP 200 OK response - slow response on second request - connection not put to idle pool",
		ClientRequestMethod:         http.MethodGet,
		ServerIdleTimeout:           5 * time.Second,
		ClientIdleTimeout:           90 * time.Second,
		ClientWaitBeforeNextReq:     1 * time.Second,
		ServerSleepBeforeResponse:   0 * time.Second,
		ServerSleepOnSecond:         true,
		ServerSleepOnSecondDuration: 1100 * time.Millisecond,
		ClientTimeout:               1 * time.Second,
		ReqCount:                    3,
		ServerType:                  ServerTypeHTTP,
	},
	{
		ID:                           "06",
		Description:                  "Server HTTP OK response for first request, but RST to a second one - retry by Round Tripper for GET",
		ClientRequestMethod:          http.MethodGet,
		ServerIdleTimeout:            5 * time.Second,
		ClientIdleTimeout:            90 * time.Second,
		ClientWaitBeforeNextReq:      2 * time.Second,
		ServerSuccessResponseOnFirst: false,
		ServerSleepBeforeResponse:    0,
		ServerSleepOnSecond:          false,
		ClientTimeout:                10 * time.Second,
		ReqCount:                     3,
		ServerType:                   ServerTypeMultiResponse,
		ServerMultiCloseConAfter:     2100 * time.Millisecond,
	},
	{
		ID:                           "07",
		Description:                  "Server HTTP OK response for first request, but RST to a second one - no retry by Round Tripper for POST",
		ClientRequestMethod:          http.MethodPost,
		ServerIdleTimeout:            5 * time.Second,
		ClientIdleTimeout:            90 * time.Second,
		ClientWaitBeforeNextReq:      2 * time.Second,
		ServerSuccessResponseOnFirst: false,
		ServerSleepBeforeResponse:    0,
		ServerSleepOnSecond:          false,
		ClientTimeout:                10 * time.Second,
		ReqCount:                     3,
		ServerType:                   ServerTypeMultiResponse,
		ServerMultiCloseConAfter:     2100 * time.Millisecond,
	},
	{
		ID:                           "08",
		Description:                  "Server HTTP OK response for first request, then closes the conn with RST before the second is closed - client detects closed connection when trying to use it and opens a new one",
		ClientRequestMethod:          http.MethodGet,
		ServerIdleTimeout:            5 * time.Second,
		ClientIdleTimeout:            90 * time.Second,
		ClientWaitBeforeNextReq:      2 * time.Second,
		ServerSuccessResponseOnFirst: false,
		ServerSleepBeforeResponse:    0,
		ServerSleepOnSecond:          false,
		ClientTimeout:                10 * time.Second,
		ReqCount:                     3,
		ServerType:                   ServerTypeMultiResponse,
		ServerMultiCloseConAfter:     1000 * time.Millisecond,
	},
	// Additional scenarios will be added here
	//  - no response at all on the server...
}

func (s Simulations) get(id string) (c *Config, found bool) {
	for _, cfg := range s {
		if cfg.ID == id {
			return &cfg, true
		}
	}
	return nil, false
}

func main() {
	simulationID, cfg, err := parseArguments()
	if err != nil {
		displayHelp()
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		return
	}
	cfg = loadConfiguration(simulationID, cfg)
	cfg.print()
	runSimulation(cfg)
}

// parseArguments handles command-line argument parsing and validation.
func parseArguments() (string, *Config, error) {
	var simulationID string
	var method string
	cfg := &Config{}

	flag.StringVar(&simulationID, "sim", "01", "Simulation scenario ID")
	flag.BoolVar(&cfg.UseHTTPS, "https", false, "Enable HTTPS")
	flag.StringVar(&method, "method", "GET", "Ad hoc change of HTTP request method (GET, POST, DELETE, HEAD)")
	flag.Usage = displayHelp
	flag.Parse()

	// Validate simulationID
	if simulationID == "" {
		return "", nil, fmt.Errorf("simulationID cannot be empty")
	}

	// Validate method
	if !isValidMethod(method) {
		return "", nil, fmt.Errorf("invalid method: %s. Allowed methods are GET, POST, DELETE, HEAD", method)
	}
	cfg.ClientRequestMethod = method

	return simulationID, cfg, nil
}

// isValidMethod checks if the provided method is one of the allowed HTTP methods.
func isValidMethod(method string) bool {
	allowedMethods := map[string]bool{
		"GET":    true,
		"POST":   true,
		"DELETE": true,
		"HEAD":   true,
	}
	return allowedMethods[method]
}

// loadConfiguration loads the appropriate configuration based on the parsed arguments.
func loadConfiguration(simulationID string, argsCfg *Config) *Config {
	cfg, exists := simulations.get(simulationID)
	if !exists {
		fmt.Printf("Invalid simulation ID: %s\n\n", simulationID)
		displayHelp()
		os.Exit(1)
	}
	if cfg.ServerType != ServerTypeHTTP && argsCfg.UseHTTPS {
		fmt.Printf("HTTPS is not supported by the selected scenario: %s\n\n", simulationID)
		os.Exit(1)
	}
	if argsCfg.ClientRequestMethod != "" {
		cfg.ClientRequestMethod = argsCfg.ClientRequestMethod
	}
	cfg.UseHTTPS = argsCfg.UseHTTPS

	// Set URLs based on HTTPS mode
	if cfg.UseHTTPS {
		cfg.ServerAddress = srvIP + ":" + portTLS
		cfg.ClientRequestURL = "https://" + cfg.ServerAddress
	} else {
		cfg.ServerAddress = srvIP + ":" + port
		cfg.ClientRequestURL = "http://" + cfg.ServerAddress
	}

	return cfg
}

// displayHelp prints the help message with available options and scenarios.
func displayHelp() {
	fmt.Println("Usage: go run main.go [OPTIONS]")
	fmt.Println("\nOptions:")
	fmt.Println("  -sim        Simulation scenario ID (e.g., '01')")
	fmt.Println("  -https      Run the selected simulation with HTTPS server (not supported by all scenarios)")
	fmt.Println("  -method     Ad hoc change of HTTP request method (GET, POST, DELETE, HEAD)")
	fmt.Println("  -h          Show this help message and exit")
	fmt.Println("\nAvailable Scenarios:")

	for _, cfg := range simulations {
		fmt.Printf("  %s: %s %s\n", cfg.ID, cfg.Description, httpsInfo(&cfg))
	}

	fmt.Println("\nExample:")
	fmt.Println("  go run main.go -sim 01")
	fmt.Println("")
}

func httpsInfo(cfg *Config) string {
	if cfg.ServerType == ServerTypeHTTP {
		return "(can HTTPS)"
	}
	return "(no HTTPS)"
}

// runSimulation sets up and executes the simulation by starting the server and client.
func runSimulation(cfg *Config) {
	var wg sync.WaitGroup

	go startServer(cfg)

	wg.Add(1)
	go func() {
		defer wg.Done()
		// wait a bit for the server to be ready
		time.Sleep(1 * time.Second)
		startClient(cfg)
	}()

	// Create a channel to listen for interrupt signals
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM) // press Ctrl+C to stop

	// Create a done channel to signal when the client finishes
	doneChan := make(chan struct{})

	// Start a goroutine to wait for the WaitGroup to finish
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case <-stopChan: // Handle Ctrl+C or termination signal
		fmt.Printf("\nprogram: shutdown signal received. Exiting...\n")
	case <-doneChan: // Goroutines finished normally
		fmt.Printf("\nprogram: all tasks completed successfully. Exiting...\n")
	}

	fmt.Println("Simulation stopped.")
}

func startServer(cfg *Config) {
	if cfg.ServerType == ServerTypeHTTP {
		startHTTPServer(cfg)
	} else {
		startTPCServer(cfg)
	}
}

func startHTTPServer(cfg *Config) {
	http.HandleFunc("/", newHandler(cfg)) // Endpoint to handle requests

	// Start the server and listen on port 8080
	server := &http.Server{
		Addr: cfg.ServerAddress,

		// Set idle timeout to simulate server closing connection after being idle
		IdleTimeout: cfg.ServerIdleTimeout,
	}

	fmt.Println("Starting server on " + cfg.ServerAddress)

	var err error
	if cfg.UseHTTPS {
		err = server.ListenAndServeTLS("server.crt", "server.key")
	} else {
		err = server.ListenAndServe()
	}
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}

func newHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		queryNum := r.FormValue("req")
		fmt.Printf("server: handling request %s num %s\n", r.Method, queryNum)

		// Handle server sleep based on the request number
		if cfg.ServerSleepOnSecond && queryNum == "2" {
			fmt.Printf("server: sleeping %d millis\n", cfg.ServerSleepOnSecondDuration.Milliseconds())
			time.Sleep(cfg.ServerSleepOnSecondDuration)
		}

		if cfg.ServerSleepBeforeResponse != 0 {
			fmt.Printf("server: sleeping %d sec\n", int(cfg.ServerSleepBeforeResponse.Seconds()))
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

func startTPCServer(cfg *Config) {
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

var connectionCount int32

func handleConnection(conn net.Conn, cfg *Config) {
	// Increment connection counter atomically
	currentConnection := atomic.AddInt32(&connectionCount, 1)

	// Success on 1st, 3rd, ... (allows simulating retry by round tripper)
	if cfg.ServerSuccessResponseOnFirst && currentConnection%2 == 1 {
		// Send HTTP 200 OK response
		sendHTTPResponse(conn, cfg)
		return
	}

	switch cfg.ServerType {
	case ServerTypeMultiResponse:
		sendMulti(conn, cfg)
		return
	case ServerTypeRST:
		sendRST(conn, cfg)
		return
	default:
		panic("unknown server type")
	}
}

func sendHTTPResponse(conn net.Conn, cfg *Config) {
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
func sendMulti(conn net.Conn, cfg *Config) {
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

func sendRST(conn net.Conn, cfg *Config) {
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

func startClient(cfg *Config) {
	// Create custom transport with idle timeout settings
	transport := &http.Transport{
		IdleConnTimeout: cfg.ClientIdleTimeout,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Allows self-signed certificates
		},
	}

	// Create an HTTP client with the custom transport
	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.ClientTimeout,
	}

	// Perform GET requests
	for i := 1; i <= cfg.ReqCount; i++ {
		sendRequest(client, i, *cfg)
		if i < cfg.ReqCount {
			wait(int(cfg.ClientWaitBeforeNextReq.Seconds()))
		}
	}
}

func wait(sec int) {
	// Simulate waiting for the server to close the connection (e.g., server idle timeout < 90s)
	fmt.Printf("client: waiting %d sec before sending the next request", sec)
	for i := 0; i < sec; i++ {
		fmt.Printf(".")
		time.Sleep(time.Second)
	}
	fmt.Println()
}

func sendRequest(client *http.Client, num int, cfg Config) {
	fmt.Printf("\nclient: Sending %d. %s request...\n", num, cfg.ClientRequestMethod)
	var err error

	req, err := http.NewRequest(cfg.ClientRequestMethod, fmt.Sprintf("%s?method=%s&req=%d", cfg.ClientRequestURL, cfg.ClientRequestMethod, num), nil)
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), getTrace())) // attach the trace to the request context

	if err != nil {
		fmt.Printf("client: Error creating request %d. request: %v\n", num, err)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("client: Error on %d. request: %v\n", num, err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("client: Response from %d. request: status: %s, body: %s\n", num, resp.Status, string(body))
}

func getTrace() *httptrace.ClientTrace {
	// Create a context with tracing
	trace := &httptrace.ClientTrace{
		GetConn: func(hostPort string) {
			fmt.Printf("client trace: Trying to get a connection for %s\n", hostPort)
		},
		GotConn: func(info httptrace.GotConnInfo) {
			fmt.Printf("client trace: Got a connection: reused=%v, wasIdle=%v, idleTime=%v\n",
				info.Reused, info.WasIdle, info.IdleTime)
		},
		PutIdleConn: func(err error) {
			if err != nil {
				fmt.Printf("client trace: Failed to put connection back to idle pool: %v\n", err)
			} else {
				fmt.Println("client trace: Connection returned to idle pool")
			}
		},
		ConnectStart: func(network, addr string) {
			fmt.Printf("client trace: Dialing new connection to %s:%s\n", network, addr)
		},
		ConnectDone: func(network, addr string, err error) {
			if err != nil {
				fmt.Printf("client trace: Failed to connect to %s:%s: %v\n", network, addr, err)
			} else {
				fmt.Printf("client trace: Successfully connected to %s:%s\n", network, addr)
			}
		},
	}
	return trace
}
