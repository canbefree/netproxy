package commands

import (
	"fmt"
	"log"
	"net"

	"github.com/spf13/cobra"
)

var (
	serverAddr string
	targetPort string
	localPort  string
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Run the client for port forwarding",
	Long:  `Run the client that connects to the server and forwards traffic between local ports and the server.`,
	Run: func(cmd *cobra.Command, args []string) {
		runClient()
	},
}

func init() {
	rootCmd.AddCommand(clientCmd)
	clientCmd.Flags().StringVarP(&serverAddr, "server", "s", "", "Server address (e.g., example.com:8000)")
	clientCmd.Flags().StringVarP(&targetPort, "target-port", "t", "8080", "Local port to forward")
	clientCmd.Flags().StringVarP(&localPort, "local-port", "l", "8080", "Local port to forward (deprecated, use --target-port)")
}

func runClient() {
	if serverAddr == "" {
		log.Fatal("Server address is required")
	}

	// Use target-port, fallback to local-port for compatibility
	if targetPort == "8080" && localPort != "8080" {
		targetPort = localPort
	}

	log.Printf("Connecting to server: %s", serverAddr)
	log.Printf("Forwarding local port %s to server", targetPort)

	for {
		conn, err := net.Dial("tcp", serverAddr)
		if err != nil {
			log.Printf("Failed to connect to server: %v", err)
			// Wait and retry
			continue
		}

		// Send the target port to server (null-terminated, 4 bytes)
		portBuf := make([]byte, 4)
		copy(portBuf, []byte(targetPort))
		conn.Write(portBuf)

		// Wait for READY signal from server
		readyBuf := make([]byte, 5)
		_, err = conn.Read(readyBuf)
		if err != nil {
			log.Printf("Failed to read READY signal: %v", err)
			conn.Close()
			continue
		}

		if string(readyBuf) != "READY" {
			log.Printf("Invalid READY signal: %s", string(readyBuf))
			conn.Close()
			continue
		}

		log.Printf("Connected to server, ready to forward traffic")

		// Handle this connection in a goroutine
		go handleClientConnection(conn)

		// Keep the main connection open
		// The server will use this to know the client is alive
		// We'll just read from it and ignore the data
		go func() {
			buf := make([]byte, 1024)
			for {
				_, err := conn.Read(buf)
				if err != nil {
					log.Printf("Main connection closed: %v", err)
					conn.Close()
					break
				}
			}
		}()

		// Wait a bit before creating a new connection for tunnels
		// This is just to prevent too many connections at once
		// The actual tunnel connections will be handled by the server
		break
	}

	// Now wait for tunnel connections from server
	for {
		conn, err := net.Dial("tcp", serverAddr)
		if err != nil {
			log.Printf("Failed to connect to server for tunnel: %v", err)
			// Wait and retry
			continue
		}

		go handleTunnelConnection(conn)
	}
}

func handleClientConnection(conn net.Conn) {
	defer conn.Close()
	// This is the main connection, we just keep it open
	// The server uses this to know the client is alive
	buf := make([]byte, 1024)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			log.Printf("Main connection closed: %v", err)
			return
		}
	}
}

func handleTunnelConnection(conn net.Conn) {
	defer conn.Close()

	// Read the target port from server
	portBuf := make([]byte, 4)
	_, err := conn.Read(portBuf)
	if err != nil {
		log.Printf("Failed to read target port from server: %v", err)
		return
	}

	targetPort := string(portBuf[:len(portBuf)-1]) // Remove null terminator

	// Connect to local service
	localConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", targetPort))
	if err != nil {
		log.Printf("Failed to connect to local port %s: %v", targetPort, err)
		return
	}
	defer localConn.Close()

	// Notify server that we're ready
	conn.Write([]byte("READY"))

	log.Printf("Tunnel established for local port %s", targetPort)

	// Forward traffic between server and local service
	forwardTraffic(conn, localConn)
}
