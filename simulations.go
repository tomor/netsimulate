package main

import (
	"fmt"
	"net/http"
	"time"
)

// Config defines simulation behaviour.
// It's also used for program arguments.
type Config struct {
	ID             string // Simulation identifier
	Description    string // Description of the scenario
	KeyLogFilePath string // program argument with path where to store TLS session keys

	ServerAddress string
	UseHTTP2      bool // Supported only by ServerTypeHTTP, enables HTTPS
	UseTLS        bool // Enable HTTPS

	ServerType        ServerType    // Type of server simulation (e.g., RST, Abrupt close)
	ServerIdleTimeout time.Duration // Server idle timeout

	// ServerTypeHTTP
	ServerSleepBeforeResponse   time.Duration // Server sleep duration to simulate delay
	ServerSleepOnSecond         bool          // Simulate sleep only on second request
	ServerSleepOnSecondDuration time.Duration // Duration of sleep on second request
	// ServerTypeRST, ServerTypeMultiResponse
	ServerSuccessResponseOnFirst bool // If the server responds with HTTP 200 OK for the first request
	// ServerTypeMultiResponse
	ServerMultiCloseConAfter time.Duration // Duration after which the connection is closed after the first response for ServerTypeMultiResponse

	ClientRequestMethod     string        // HTTP request type (GET, POST, etc.)
	ClientRequestURL        string        // URL to which the client sends the HTTP request
	ClientWaitBeforeNextReq time.Duration // Time client waits before next request
	ReqInParallel           bool          // When true, requests can be done in parallel
	ClientIdleTimeout       time.Duration // Time for which the TCP connection is kept in the idle pool
	ClientMaxConnsPerHost   int           // http.Transport.MaxConnsPerHost, default 0
	ClientTimeout           time.Duration // Client timeout for each request
	ReqCount                int           // Number of requests client will make
}

// print outputs simulation configuration.
// It's printed at the beginning to stdout.
func (c Config) print() {
	fmt.Println("Configuration:")
	fmt.Printf("  ID:                           %s\n", c.ID)
	fmt.Printf("  Description:                  %s\n", c.Description)
	fmt.Printf("  Server Address:               %s\n", c.ServerAddress)
	fmt.Printf("  Use HTTP2:                    %v\n", c.UseHTTP2)
	fmt.Printf("  Use TLS:                      %t\n", c.UseTLS)
	fmt.Printf("  Server Idle Timeout:          %.0f sec\n", c.ServerIdleTimeout.Seconds())
	fmt.Printf("  Server Success On First:      %v\n", c.ServerSuccessResponseOnFirst)
	fmt.Printf("  Server Sleep Before Response: %.1f sec\n", c.ServerSleepBeforeResponse.Seconds())
	fmt.Printf("  Server Sleep On Second:       %t\n", c.ServerSleepOnSecond)
	fmt.Printf("  Server Sleep On Second Dur:   %.1f sec\n", c.ServerSleepOnSecondDuration.Seconds())
	fmt.Printf("  Client Request Type:          %s\n", c.ClientRequestMethod)
	fmt.Printf("  Client Idle Timeout:          %.0f sec\n", c.ClientIdleTimeout.Seconds())
	fmt.Printf("  Client MaxConnsPerHost:       %d %s\n", c.ClientMaxConnsPerHost, infoMaxConns(c.ClientMaxConnsPerHost))
	fmt.Printf("  Client Wait Before Next Req:  %.1f sec\n", c.ClientWaitBeforeNextReq.Seconds())
	fmt.Printf("  Client Timeout:               %.0f sec\n", c.ClientTimeout.Seconds())
	fmt.Printf("  Request Count:                %d\n", c.ReqCount)
	fmt.Printf("  Requests In Parallel:         %v\n", c.ReqInParallel)
	fmt.Println()
}

type Simulations []Config

// list of available simulations
var simulations = Simulations{
	{
		ID:                        "01",
		Description:               "Server HTTP 200 OK response - connection reused from idle pool",
		ServerType:                ServerTypeHTTP,
		ServerIdleTimeout:         5 * time.Second,
		ServerSleepBeforeResponse: 0,
		ServerSleepOnSecond:       false,
		ClientRequestMethod:       http.MethodGet,
		ClientIdleTimeout:         90 * time.Second,
		ClientWaitBeforeNextReq:   1 * time.Second,
		ClientTimeout:             10 * time.Second,
		ReqCount:                  3,
	},
	{
		ID:                        "02",
		Description:               "Server HTTP 200 OK response - connection not reused from idle pool - over server idle timeout",
		ServerType:                ServerTypeHTTP,
		ServerIdleTimeout:         1 * time.Second,
		ServerSleepBeforeResponse: 0,
		ServerSleepOnSecond:       false,
		ClientRequestMethod:       http.MethodGet,
		ClientIdleTimeout:         90 * time.Second,
		ClientWaitBeforeNextReq:   1100 * time.Millisecond,
		ClientTimeout:             10 * time.Second,
		ReqCount:                  3,
	},
	{
		ID:                        "03",
		Description:               "Server HTTP 200 OK response - connection not reused from idle pool - over client idle timeout",
		ServerType:                ServerTypeHTTP,
		ServerIdleTimeout:         5 * time.Second,
		ServerSleepBeforeResponse: 0,
		ServerSleepOnSecond:       false,
		ClientRequestMethod:       http.MethodGet,
		ClientIdleTimeout:         1 * time.Second,
		ClientWaitBeforeNextReq:   2 * time.Second,
		ClientTimeout:             10 * time.Second,
		ReqCount:                  3,
	},
	{
		ID:                        "04",
		Description:               "Server HTTP 200 OK response - slow response - client timeout - connection not put to idle pool",
		ServerType:                ServerTypeHTTP,
		ServerIdleTimeout:         5 * time.Second,
		ServerSleepBeforeResponse: 1100 * time.Millisecond,
		ServerSleepOnSecond:       false,
		ClientRequestMethod:       http.MethodGet,
		ClientIdleTimeout:         90 * time.Second,
		ClientWaitBeforeNextReq:   1 * time.Second,
		ClientTimeout:             1 * time.Second,
		ReqCount:                  3,
	},
	{
		ID:                          "05",
		Description:                 "Server HTTP 200 OK response - slow response on second request - connection not put to idle pool",
		ServerType:                  ServerTypeHTTP,
		ServerIdleTimeout:           5 * time.Second,
		ServerSleepBeforeResponse:   0 * time.Second,
		ServerSleepOnSecond:         true,
		ServerSleepOnSecondDuration: 1100 * time.Millisecond,
		ClientRequestMethod:         http.MethodGet,
		ClientIdleTimeout:           90 * time.Second,
		ClientWaitBeforeNextReq:     1 * time.Second,
		ClientTimeout:               1 * time.Second,
		ReqCount:                    3,
	},
	{
		ID:                           "06",
		Description:                  "Server HTTP OK response for first request, but RST to a second one - retry by Round Tripper for GET",
		ServerType:                   ServerTypeMultiResponse,
		ServerMultiCloseConAfter:     2100 * time.Millisecond,
		ServerIdleTimeout:            5 * time.Second,
		ServerSuccessResponseOnFirst: false,
		ServerSleepBeforeResponse:    0,
		ServerSleepOnSecond:          false,
		ClientRequestMethod:          http.MethodGet,
		ClientIdleTimeout:            90 * time.Second,
		ClientWaitBeforeNextReq:      2 * time.Second,
		ClientTimeout:                10 * time.Second,
		ReqCount:                     3,
	},
	{
		ID:                           "07",
		Description:                  "Server HTTP OK response for first request, but RST to a second one - no retry by Round Tripper for POST",
		ServerType:                   ServerTypeMultiResponse,
		ServerMultiCloseConAfter:     2100 * time.Millisecond,
		ServerIdleTimeout:            5 * time.Second,
		ServerSuccessResponseOnFirst: false,
		ServerSleepBeforeResponse:    0,
		ServerSleepOnSecond:          false,
		ClientRequestMethod:          http.MethodPost,
		ClientIdleTimeout:            90 * time.Second,
		ClientWaitBeforeNextReq:      2 * time.Second,
		ClientTimeout:                10 * time.Second,
		ReqCount:                     3,
	},
	{
		ID:                           "08",
		Description:                  "Server HTTP OK response for first request, then closes the conn with RST before the second is closed - client detects closed connection when trying to use it and opens a new one",
		ServerType:                   ServerTypeMultiResponse,
		ServerMultiCloseConAfter:     1000 * time.Millisecond,
		ServerIdleTimeout:            5 * time.Second,
		ServerSuccessResponseOnFirst: false,
		ServerSleepBeforeResponse:    0,
		ServerSleepOnSecond:          false,
		ClientRequestMethod:          http.MethodGet,
		ClientIdleTimeout:            90 * time.Second,
		ClientWaitBeforeNextReq:      2 * time.Second,
		ClientTimeout:                10 * time.Second,
		ReqCount:                     3,
	},
	{
		ID:                           "09",
		Description:                  "Multiple requests in parallel - multiple TCP connections",
		ServerType:                   ServerTypeHTTP,
		ServerIdleTimeout:            5 * time.Second,
		ServerSuccessResponseOnFirst: false,
		ServerSleepBeforeResponse:    10 * time.Millisecond,
		ServerSleepOnSecond:          false,
		ClientRequestMethod:          http.MethodGet,
		ClientIdleTimeout:            90 * time.Second,
		ClientWaitBeforeNextReq:      0,
		ReqInParallel:                true,
		ClientTimeout:                10 * time.Second,
		ReqCount:                     3,
	},
	{
		ID:                           "10",
		Description:                  "Multiple requests in parallel with client config MaxConnsPerHost=1 - one TCP connection is used",
		ServerType:                   ServerTypeHTTP,
		ServerSuccessResponseOnFirst: false,
		ServerSleepBeforeResponse:    10 * time.Millisecond,
		ServerSleepOnSecond:          false,
		ServerIdleTimeout:            5 * time.Second,
		ClientRequestMethod:          http.MethodGet,
		ClientIdleTimeout:            90 * time.Second,
		ClientMaxConnsPerHost:        1,
		ClientWaitBeforeNextReq:      0,
		ReqInParallel:                true,
		ClientTimeout:                10 * time.Second,
		ReqCount:                     3,
	},
	{
		ID:                      "20",
		Description:             "HTTP2, Server HTTP 200 OK response - connection reused from idle pool",
		UseHTTP2:                true,
		UseTLS:                  true,
		ServerType:              ServerTypeHTTP,
		ServerIdleTimeout:       5 * time.Second,
		ClientRequestMethod:     http.MethodGet,
		ClientIdleTimeout:       90 * time.Second,
		ClientWaitBeforeNextReq: 1 * time.Second,
		ReqInParallel:           false,
		ClientTimeout:           10 * time.Second,
		ReqCount:                3,
	},
}

func (s Simulations) get(id string) (c *Config, found bool) {
	for _, cfg := range s {
		if cfg.ID == id {
			return &cfg, true
		}
	}
	return nil, false
}

func infoMaxConns(num int) string {
	if num == 0 {
		return "(unlimited)"
	}
	return ""
}
