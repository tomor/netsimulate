package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"sync"
	"time"
)

func wait(sec int) {
	if sec == 0 {
		return
	}
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

// create key log writer only if HTTPS was enabled and SSLKEYLOGFILE env variable is defined
func getKeyLogWriter(cfg *Config) io.Writer {
	if !cfg.UseTLS {
		return nil
	}

	if cfg.KeyLogFilePath == "" {
		return nil
	}

	file, err := os.OpenFile(cfg.KeyLogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Printf("Error: Failed to open keylog file (%s): %v", cfg.KeyLogFilePath, err)
		return nil
	}
	fmt.Println("client: logging SSL key exchange to '" + cfg.KeyLogFilePath + "'")

	return file
}

func startClient(cfg *Config) {
	keyLogFile := getKeyLogWriter(cfg)
	if keyLogFile != nil {
		if file, ok := keyLogFile.(*os.File); ok {
			defer file.Close()
		}
	}

	// Create custom transport with idle timeout settings
	transport := &http.Transport{
		IdleConnTimeout: cfg.ClientIdleTimeout,
		MaxConnsPerHost: cfg.ClientMaxConnsPerHost,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Allows self-signed certificates
			KeyLogWriter:       keyLogFile,
		},
		ForceAttemptHTTP2: cfg.UseHTTP2,
	}

	// Create an HTTP client with the custom transport
	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.ClientTimeout,
	}

	wg := sync.WaitGroup{}
	wg.Add(cfg.ReqCount)
	// Perform GET requests
	for i := 1; i <= cfg.ReqCount; i++ {
		if cfg.ReqInParallel {
			go func() {
				sendRequest(client, i, *cfg)
				wg.Done()
			}()
		} else {
			sendRequest(client, i, *cfg)
			wg.Done()
		}
		if i < cfg.ReqCount {
			wait(int(cfg.ClientWaitBeforeNextReq.Seconds()))
		}
	}
	if cfg.ReqInParallel {
		fmt.Println("client: waiting for all requests to finish")
	}
	wg.Wait()
}
