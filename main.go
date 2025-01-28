package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var version = "dev"

// Constants
const (
	BufferLen = 65535 // Maximum UDP read buffer size
)

// Global variables for command-line flags
var (
	ifacesFlag    = flag.String("i", "", "Comma-separated list of interface names (e.g. 'eth0,eth1,vlan3')")
	portsFlag     = flag.String("p", "", "Comma-separated list of UDP ports to listen on (e.g. '1900,1990')")
	groupsFlag    = flag.String("g", "", "Comma-separated list of multicast groups (e.g. '239.255.255.250,239.255.255.251')")
	destPortsFlag = flag.String("d", "", "Comma-separated list of target UDP ports to forward to (optional, e.g. '2021,2022')")
	verboseFlag   = flag.Bool("v", false, "Enable verbose/debug logging")
)

// firstIPv4Addr returns the first IPv4 address found on the given interface.
func firstIPv4Addr(ifi *net.Interface) (string, error) {
	addrs, err := ifi.Addrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil {
			continue
		}
		ipv4 := ip.To4()
		if ipv4 != nil {
			return ipv4.String(), nil
		}
	}
	return "", fmt.Errorf("no IPv4 address found on interface %s", ifi.Name)
}

// Add a --version flag
func init() {
	versionFlag := flag.Bool("version", false, "Print the version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("ssdp-forwarder version %s\n", version)
		os.Exit(0)
	}
}

func main() {
	// Parse command-line flags
	flag.Parse()

	// Setup logging based on verbose flag
	if *verboseFlag {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	} else {
		log.SetFlags(log.LstdFlags)
	}

	// Validate mandatory flags
	if *ifacesFlag == "" {
		log.Fatalf("No interfaces specified. Use -i <iface1,iface2,...>")
	}
	if *portsFlag == "" {
		log.Fatalf("No ports specified. Use -p <port1,port2,...>")
	}
	if *groupsFlag == "" {
		log.Fatalf("No groups specified. Use -g <group1,group2,...>")
	}

	// Split and parse the flags
	ifaceNames := parseCommaSeparated(*ifacesFlag)
	portStrs := parseCommaSeparated(*portsFlag)
	groupStrs := parseCommaSeparated(*groupsFlag)

	// Convert port strings to int
	ports, err := parsePorts(portStrs)
	if err != nil {
		log.Fatalf("Error parsing ports: %v", err)
	}

	// Trim spaces from group addresses
	for i, g := range groupStrs {
		groupStrs[i] = strings.TrimSpace(g)
	}

	// Handle destination ports (-d)
	var destPorts []int
	if *destPortsFlag != "" {
		destPortStrs := parseCommaSeparated(*destPortsFlag)
		destPorts, err = parsePorts(destPortStrs)
		if err != nil {
			log.Fatalf("Error parsing destination ports: %v", err)
		}
		if len(destPorts) != len(ports) {
			log.Fatalf("Number of destination ports (%d) must match number of listening ports (%d)", len(destPorts), len(ports))
		}
	} else {
		// If -d not set, use the same ports for destination
		destPorts = ports
	}

	// Initialize data structures
	// conns[group][iface][port] and senders[group][iface][port]
	conns, senders := initializeConnections(groupStrs, ifaceNames, ports, destPorts)

	// Start forwarding goroutines
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	startForwarding(ctx, &wg, conns, senders, groupStrs, ifaceNames, ports, destPorts, *verboseFlag)

	// Handle graceful shutdown
	handleShutdown(cancel)

	// Wait for all goroutines to finish
	wg.Wait()

	// Close all sockets
	closeConnections(conns, senders)

	log.Println("All done.")
}

// parseCommaSeparated splits a comma-separated string into a slice of strings.
func parseCommaSeparated(input string) []string {
	parts := strings.Split(input, ",")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// parsePorts converts a slice of port strings to a slice of integers.
func parsePorts(portStrs []string) ([]int, error) {
	var ports []int
	for _, p := range portStrs {
		var port int
		if _, err := fmt.Sscanf(p, "%d", &port); err != nil {
			return nil, fmt.Errorf("invalid port %q: %v", p, err)
		}
		if port <= 0 || port > 65535 {
			return nil, fmt.Errorf("port %d out of valid range (1-65535)", port)
		}
		ports = append(ports, port)
	}
	return ports, nil
}

// initializeConnections sets up listening and sending UDP connections.
func initializeConnections(groups, ifaces []string, ports, destPorts []int) ([][][]*net.UDPConn, [][][]*net.UDPConn) {
	numGroups := len(groups)
	numIfaces := len(ifaces)
	numPorts := len(ports)

	conns := make([][][]*net.UDPConn, numGroups)
	senders := make([][][]*net.UDPConn, numGroups)

	for g := 0; g < numGroups; g++ {
		conns[g] = make([][]*net.UDPConn, numIfaces)
		senders[g] = make([][]*net.UDPConn, numIfaces)
		for i := 0; i < numIfaces; i++ {
			conns[g][i] = make([]*net.UDPConn, numPorts)
			senders[g][i] = make([]*net.UDPConn, numPorts)
		}
	}

	for g, group := range groups {
		mcastIP := net.ParseIP(group)
		if mcastIP == nil {
			log.Fatalf("Failed to parse multicast group %q", group)
		}

		for i, ifName := range ifaces {
			ifi, err := net.InterfaceByName(ifName)
			if err != nil {
				log.Fatalf("Could not find interface %q: %v", ifName, err)
			}

			localIP, err := firstIPv4Addr(ifi)
			if err != nil {
				log.Fatalf("Could not determine IPv4 for interface %s: %v", ifName, err)
			}

			for p, port := range ports {
				maddr := &net.UDPAddr{
					IP:   mcastIP,
					Port: port,
				}

				// 1) Listen for multicast on (group, port) for this interface
				lconn, err := net.ListenMulticastUDP("udp4", ifi, maddr)
				if err != nil {
					log.Fatalf("Failed to listen on group=%s, port=%d, iface=%s: %v",
						group, port, ifName, err)
				}

				// Optionally adjust read buffer size
				lconn.SetReadBuffer(BufferLen)

				// 2) Create sending connection from (localIP) to (group:destPort)
				destPort := destPorts[p]
				destAddr := &net.UDPAddr{
					IP:   mcastIP,
					Port: destPort,
				}
				localAddr := &net.UDPAddr{IP: net.ParseIP(localIP), Port: 0} // Ephemeral port
				senderConn, err := net.DialUDP("udp4", localAddr, destAddr)
				if err != nil {
					log.Fatalf("Could not create sender on group=%s, iface=%s (%s), port=%d: %v",
						group, ifName, localIP, destPort, err)
				}

				conns[g][i][p] = lconn
				senders[g][i][p] = senderConn

				log.Printf("Joined group=%s on interface=%s:%d, localIP=%s (listening & sending to port %d)",
					group, ifName, port, localIP, destPort)
			}
		}
	}

	return conns, senders
}

// startForwarding launches goroutines to handle packet forwarding.
func startForwarding(ctx context.Context, wg *sync.WaitGroup, conns, senders [][][]*net.UDPConn, groups, ifaces []string, ports, destPorts []int, verbose bool) {
	for g := range groups {
		for i := range ifaces {
			for p := range ports {
				wg.Add(1)
				go func(gIdx, iIdx, pIdx int) {
					defer wg.Done()
					buf := make([]byte, BufferLen)
					lconn := conns[gIdx][iIdx][pIdx]
					group := groups[gIdx]
					port := ports[pIdx]
					// destPort := destPorts[pIdx]
					ifaceName := ifaces[iIdx]

					for {
						select {
						case <-ctx.Done():
							if verbose {
								log.Printf("Goroutine for group=%s, iface=%s, port=%d exiting.", group, ifaceName, port)
							}
							return
						default:
							// Set a deadline to allow goroutine to exit on context cancellation
							lconn.SetReadDeadline(time.Now().Add(1 * time.Second))
							n, src, err := lconn.ReadFromUDP(buf)
							if err != nil {
								// Check if timeout due to deadline
								if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
									continue // Retry reading
								}
								log.Printf("Read error on group=%s, iface=%s, port=%d: %v",
									group, ifaceName, port, err)
								return
							}

							packet := make([]byte, n)
							copy(packet, buf[:n])

							// Forward to all other interfaces for the same group & port
							for otherIfIdx := range ifaces {
								if otherIfIdx == iIdx {
									continue // Don't send back on the same interface
								}
								senderConn := senders[gIdx][otherIfIdx][pIdx]
								_, werr := senderConn.Write(packet)
								if werr != nil {
									log.Printf("Forward error: group=%s, from iface=%s to iface=%s, dest port=%d: %v",
										group, ifaces[iIdx], ifaces[otherIfIdx], destPorts[pIdx], werr)
								} else if verbose {
									log.Printf("Forwarded %d bytes from %s:%d on iface=%s to iface=%s:%d",
										n, src.IP, src.Port, ifaces[iIdx], ifaces[otherIfIdx], destPorts[pIdx])
								}
							}

							if verbose {
								log.Printf("Received %d bytes from %v on (group=%s, iface=%s, port=%d)",
									n, src, group, ifaceName, port)
							}
						}
					}
				}(g, i, p)
			}
		}
	}
}

// handleShutdown sets up signal handling for graceful shutdown.
func handleShutdown(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %s. Shutting down...", sig)
		cancel()
	}()
}

// closeConnections gracefully closes all UDP connections.
func closeConnections(conns, senders [][][]*net.UDPConn) {
	for g := range conns {
		for i := range conns[g] {
			for p := range conns[g][i] {
				if conns[g][i][p] != nil {
					conns[g][i][p].Close()
				}
				if senders[g][i][p] != nil {
					senders[g][i][p].Close()
				}
			}
		}
	}
}