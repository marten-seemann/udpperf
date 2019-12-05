package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	// "net/http"
	"strconv"
	"time"

	_ "net/http/pprof"
)

const defaultPort = 5201

func parseBytes(s string) (int64, error) {
	unit := s[len(s)-1]
	if unit != 'k' && unit != 'm' && unit != 'g' {
		return strconv.ParseInt(s, 10, 64)
	}
	num, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
	if err != nil {
		return 0, err
	}
	switch unit {
	case 'k':
		return num * 1 << 10, nil
	case 'm':
		return num * 1 << 20, nil
	case 'g':
		return num * 1 << 30, nil
	default:
		panic("invalid unit")
	}
}

func main() {
	// enable pprof profiling
	// go func() {
	// 	log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
	// }()

	server := flag.Bool("s", false, "run as server")
	client := flag.String("c", "", "run as client: remote address")
	port := flag.Int("p", defaultPort, "port")
	seconds := flag.Int("t", 10, "time in seconds")
	packetSize := flag.Int("l", 1250, "UDP payload size")
	bandwidthStr := flag.String("b", "1m", "bandwidth")
	flag.Parse()

	duration := time.Duration(*seconds) * time.Second
	bandwidth, err := parseBytes(*bandwidthStr)
	if err != nil {
		log.Fatalf("Invalid bandwidth: %s", err.Error())
	}

	if *server {
		err = runServer(*port)
	} else {
		err = runClient(*client, *port, duration, *packetSize, bandwidth)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func humanizeBytes(s uint64) string {
	return humanateBytes(s, []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"})
}

func humanizeBits(s uint64) string {
	return humanateBytes(s, []string{"Bits", "kBits", "MBits", "GBits", "TBits", "PBits", "EBits"})
}

// see https://github.com/dustin/go-humanize/blob/master/bytes.go
func humanateBytes(s uint64, sizes []string) string {
	const base = 1000
	if s < 10 {
		return fmt.Sprintf("%d B", s)
	}
	e := math.Floor(math.Log(float64(s)) / math.Log(base))
	suffix := sizes[int(e)]
	val := math.Floor(float64(s)/math.Pow(base, e)*10+0.5) / 10
	f := "%.2f %s"
	if val < 10 {
		f = "%.2f %s"
	}

	return fmt.Sprintf(f, val, suffix)
}

func runServer(port int) error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return err
	}
	fmt.Println("-----------------------------------------------------------")
	fmt.Printf("Server listening on %d\n", port)
	fmt.Println("-----------------------------------------------------------")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	const bufSize = 4 * 1 << 10
	b := make([]byte, bufSize)
	var highest uint64
	for {
		b = b[:bufSize]
		n, _, err := conn.ReadFrom(b)
		if err != nil {
			return err
		}
		b = b[:n]
		pn := binary.BigEndian.Uint64(b[:8])
		// ignore reordered packets
		if pn <= highest {
			continue
		}
		// report gaps
		if pn != highest+1 {
			if pn-highest > 2 {
				fmt.Printf("Lost packets between %d and %d\n", highest+1, pn-1)
			} else {
				fmt.Printf("Lost packet %d\n", highest+1)
			}
		}
		highest = pn
	}
}

func runClient(address string, port int, duration time.Duration, packetSize int, bandwidth int64) error {
	fmt.Printf("Connecting to host %s, port %d\n", address, port)
	pps := bandwidth / int64(packetSize)
	fmt.Printf("Sending %d packets (%d bytes) per second\n", pps, packetSize)

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		return err
	}
	laddr, err := net.ResolveUDPAddr("udp", "localhost:0")
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", laddr, addr)
	if err != nil {
		return err
	}

	b := make([]byte, packetSize)
	ticker := time.NewTicker(time.Second / time.Duration(pps))
	timer := time.NewTimer(duration)
	reporter := time.NewTicker(time.Second)
	var pn uint64
	var highestReported uint64
	for {
		select {
		case <-reporter.C:
			fmt.Printf("\tSent %d packets.\n", pn-highestReported)
			highestReported = pn
			continue
		case <-timer.C:
			return nil
		case <-ticker.C:
		}
		pn++
		binary.BigEndian.PutUint64(b[:8], pn)
		if _, err := conn.Write(b); err != nil {
			return err
		}
	}
}
