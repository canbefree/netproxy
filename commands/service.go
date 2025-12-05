package commands

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/spf13/cobra"
)

var (
	servicePort string
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Run the server service for port forwarding",
	Long:  `Run the server service that listens for client connections and forwards traffic between clients and public ports.`,
	Run: func(cmd *cobra.Command, args []string) {
		runService()
	},
}

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.Flags().StringVarP(&servicePort, "port", "p", "8000", "Port to listen on")
}

func runService() {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", servicePort))
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", servicePort, err)
	}
	defer listener.Close()

	log.Printf("Server listening on port %s", servicePort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go handleServiceConnection(conn)
	}
}

func handleServiceConnection(conn net.Conn) {
	defer conn.Close()

	log.Printf("New client connected: %s", conn.RemoteAddr().String())

	// Read the target port from client
	portBuf := make([]byte, 4)
	_, err := conn.Read(portBuf)
	if err != nil {
		log.Printf("Failed to read target port: %v", err)
		return
	}

	targetPort := string(portBuf[:len(portBuf)-1]) // Remove null terminator

	// Listen on the target port
	localListener, err := net.Listen("tcp", fmt.Sprintf(":%s", targetPort))
	if err != nil {
		log.Printf("Failed to listen on target port %s: %v", targetPort, err)
		return
	}
	defer localListener.Close()

	log.Printf("Listening on target port %s for client %s", targetPort, conn.RemoteAddr().String())

	// Notify client that the port is ready
	conn.Write([]byte("READY"))

	// Accept connections on the target port and forward to client
	for {
		localConn, err := localListener.Accept()
		if err != nil {
			log.Printf("Failed to accept local connection: %v", err)
			break
		}

		go func(localConn net.Conn) {
			defer localConn.Close()

			// Create a new connection to client for this tunnel
			clientConn, err := net.Dial("tcp", conn.RemoteAddr().String())
			if err != nil {
				log.Printf("Failed to reconnect to client: %v", err)
				return
			}
			defer clientConn.Close()

			// Send the target port again for this tunnel
			clientConn.Write(append([]byte(targetPort), 0))

			// Wait for READY signal
			readyBuf := make([]byte, 5)
			_, err = clientConn.Read(readyBuf)
			if err != nil || string(readyBuf) != "READY" {
				log.Printf("Failed to get READY signal from client: %v", err)
				return
			}

			// Forward traffic between local connection and client
			forwardTraffic(localConn, clientConn)
		}(localConn)
	}
}

func forwardTraffic(conn1, conn2 net.Conn) {
	var wg sync.WaitGroup

	// Forward from conn1 to conn2
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(conn2, conn1)
		conn2.Close()
	}()

	// Forward from conn2 to conn1
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(conn1, conn2)
		conn1.Close()
	}()

	wg.Wait()
}