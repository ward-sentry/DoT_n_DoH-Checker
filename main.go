package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

var appVersion = "dev"

type DoTTarget struct {
	Name string
	Host string
	IP   string
}

type DoHTarget struct {
	Name string
	URL  string
}

type Result struct {
	Protocol string
	Name     string
	Endpoint string
	OK       int
	Fail     int
	Min      time.Duration
	Median   time.Duration
	Average  time.Duration
	Max      time.Duration
	Samples  []time.Duration
	Errors   []string
}

type Config struct {
	DoTFile string
	DoHFile string
	Domain  string
	Count   int
	Timeout time.Duration
	Top     int
	Only    string
	Pause   bool
	Version bool
}

func main() {
	cfg := parseFlags()
	defer pauseBeforeExit(cfg)
	if cfg.Version {
		fmt.Printf("dotndoh-checker %s\n", appVersion)
		return
	}

	dotTargets, err := loadDoT(cfg.DoTFile)
	if err != nil {
		printErr("load DoT targets", err)
		return
	}

	dohTargets, err := loadDoH(cfg.DoHFile)
	if err != nil {
		printErr("load DoH targets", err)
		return
	}

	fmt.Printf("\nDoT / DoH Checker\n")
	fmt.Printf("Domain: %s | Attempts: %d | Timeout: %s | Top: %d\n\n", cfg.Domain, cfg.Count, cfg.Timeout, cfg.Top)

	var results []Result
	if cfg.Only == "" || cfg.Only == "dot" {
		fmt.Printf("Testing DoT targets from %s...\n", cfg.DoTFile)
		results = append(results, runDoT(dotTargets, cfg)...)
		printResults("DoT", resultsByProtocol(results, "DoT"), cfg.Top)
	}

	if cfg.Only == "" || cfg.Only == "doh" {
		fmt.Printf("\nTesting DoH targets from %s...\n", cfg.DoHFile)
		results = append(results, runDoH(dohTargets, cfg)...)
		printResults("DoH", resultsByProtocol(results, "DoH"), cfg.Top)
	}
}

func parseFlags() Config {
	cfg := Config{Pause: runtime.GOOS == "windows"}
	flag.StringVar(&cfg.DoTFile, "dot", "DoT.txt", "path to DoT target list")
	flag.StringVar(&cfg.DoHFile, "doh", "DoH.txt", "path to DoH target list")
	flag.StringVar(&cfg.Domain, "domain", "example.com", "domain to resolve during checks")
	flag.IntVar(&cfg.Count, "count", 7, "attempts per resolver")
	flag.DurationVar(&cfg.Timeout, "timeout", 3500*time.Millisecond, "timeout per attempt")
	flag.IntVar(&cfg.Top, "top", 4, "number of best resolvers to print")
	flag.StringVar(&cfg.Only, "only", "", "limit to dot or doh")
	flag.BoolVar(&cfg.Pause, "pause", cfg.Pause, "wait for Enter before exit")
	flag.BoolVar(&cfg.Version, "version", false, "print version and exit")
	flag.Parse()

	cfg.Only = strings.ToLower(strings.TrimSpace(cfg.Only))
	if cfg.Count < 1 {
		cfg.Count = 1
	}
	if cfg.Top < 1 {
		cfg.Top = 1
	}
	if cfg.Only != "" && cfg.Only != "dot" && cfg.Only != "doh" {
		fmt.Fprintln(os.Stderr, "parse flags: -only must be dot or doh")
		os.Exit(2)
	}
	return cfg
}

func loadDoT(path string) ([]DoTTarget, error) {
	lines, err := readTargetLines(path)
	if err != nil {
		return nil, err
	}
	targets := make([]DoTTarget, 0, len(lines))
	for i, line := range lines {
		parts := strings.Fields(line)
		if len(parts) != 2 && len(parts) != 3 {
			return nil, fmt.Errorf("%s:%d: expected: name host [ip]", path, i+1)
		}
		target := DoTTarget{Name: parts[0], Host: parts[1]}
		if len(parts) == 3 {
			if net.ParseIP(parts[2]) == nil {
				return nil, fmt.Errorf("%s:%d: invalid ip %q", path, i+1, parts[2])
			}
			target.IP = parts[2]
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func loadDoH(path string) ([]DoHTarget, error) {
	lines, err := readTargetLines(path)
	if err != nil {
		return nil, err
	}
	targets := make([]DoHTarget, 0, len(lines))
	for i, line := range lines {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%s:%d: expected: name url", path, i+1)
		}
		u, err := url.Parse(parts[1])
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, fmt.Errorf("%s:%d: invalid https url %q", path, i+1, parts[1])
		}
		targets = append(targets, DoHTarget{Name: parts[0], URL: parts[1]})
	}
	return targets, nil
}

func readTargetLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("%s has no targets", path)
	}
	return lines, nil
}

func runDoT(targets []DoTTarget, cfg Config) []Result {
	results := make([]Result, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(i int, target DoTTarget) {
			defer wg.Done()
			results[i] = measure("DoT", target.Name, dotEndpoint(target), cfg, func(ctx context.Context) error {
				return queryDoT(ctx, target, cfg.Domain)
			})
			printProgress(results[i])
		}(i, target)
	}
	wg.Wait()
	return ranked(results)
}

func runDoH(targets []DoHTarget, cfg Config) []Result {
	results := make([]Result, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(i int, target DoHTarget) {
			defer wg.Done()
			results[i] = measure("DoH", target.Name, target.URL, cfg, func(ctx context.Context) error {
				return queryDoH(ctx, target, cfg.Domain)
			})
			printProgress(results[i])
		}(i, target)
	}
	wg.Wait()
	return ranked(results)
}

func measure(protocol, name, endpoint string, cfg Config, query func(context.Context) error) Result {
	result := Result{Protocol: protocol, Name: name, Endpoint: endpoint}
	for i := 0; i < cfg.Count; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		start := time.Now()
		err := query(ctx)
		elapsed := time.Since(start)
		cancel()

		if err != nil {
			result.Fail++
			if len(result.Errors) < 3 {
				result.Errors = append(result.Errors, compactErr(err))
			}
			continue
		}

		result.OK++
		result.Samples = append(result.Samples, elapsed)
		time.Sleep(120 * time.Millisecond)
	}
	summarize(&result)
	return result
}

func queryDoT(ctx context.Context, target DoTTarget, domain string) error {
	query, err := buildDNSQuery(domain)
	if err != nil {
		return err
	}

	dialer := &net.Dialer{}
	dialHost := target.Host
	if target.IP != "" {
		dialHost = target.IP
	}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(dialHost, "853"))
	if err != nil {
		return err
	}
	defer conn.Close()

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName: target.Host,
		MinVersion: tls.VersionTLS12,
	})
	if deadline, ok := ctx.Deadline(); ok {
		_ = tlsConn.SetDeadline(deadline)
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return err
	}

	var frame bytes.Buffer
	_ = binary.Write(&frame, binary.BigEndian, uint16(len(query)))
	frame.Write(query)
	if _, err := tlsConn.Write(frame.Bytes()); err != nil {
		return err
	}

	var size uint16
	if err := binary.Read(tlsConn, binary.BigEndian, &size); err != nil {
		return err
	}
	if size < 12 {
		return errors.New("short dns response")
	}
	response := make([]byte, size)
	if _, err := io.ReadFull(tlsConn, response); err != nil {
		return err
	}
	return validateDNSResponse(response)
}

func queryDoH(ctx context.Context, target DoHTarget, domain string) error {
	query, err := buildDNSQuery(domain)
	if err != nil {
		return err
	}
	encoded := base64.RawURLEncoding.EncodeToString(query)

	u, err := url.Parse(target.URL)
	if err != nil {
		return err
	}
	params := u.Query()
	params.Set("dns", encoded)
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/dns-message")

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return validateDNSResponse(body)
}

func buildDNSQuery(domain string) ([]byte, error) {
	domain = strings.TrimSuffix(strings.TrimSpace(domain), ".")
	if domain == "" {
		return nil, errors.New("empty domain")
	}

	buf := bytes.NewBuffer(make([]byte, 0, 64))
	var id [2]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	buf.Write(id[:])
	buf.Write([]byte{0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 {
			return nil, fmt.Errorf("invalid domain label %q", label)
		}
		buf.WriteByte(byte(len(label)))
		buf.WriteString(label)
	}
	buf.Write([]byte{0x00, 0x00, 0x01, 0x00, 0x01})
	return buf.Bytes(), nil
}

func validateDNSResponse(response []byte) error {
	if len(response) < 12 {
		return errors.New("short dns response")
	}
	rcode := response[3] & 0x0f
	if rcode != 0 {
		return fmt.Errorf("dns rcode %d", rcode)
	}
	return nil
}

func dotEndpoint(target DoTTarget) string {
	if target.IP == "" {
		return target.Host + ":853"
	}
	return fmt.Sprintf("%s (%s:853)", target.Host, target.IP)
}

func summarize(result *Result) {
	if len(result.Samples) == 0 {
		return
	}
	sort.Slice(result.Samples, func(i, j int) bool { return result.Samples[i] < result.Samples[j] })
	result.Min = result.Samples[0]
	result.Max = result.Samples[len(result.Samples)-1]
	result.Median = result.Samples[len(result.Samples)/2]

	var total time.Duration
	for _, sample := range result.Samples {
		total += sample
	}
	result.Average = time.Duration(int64(total) / int64(len(result.Samples)))
}

func ranked(results []Result) []Result {
	sort.Slice(results, func(i, j int) bool {
		if results[i].OK != results[j].OK {
			return results[i].OK > results[j].OK
		}
		if results[i].Median != results[j].Median {
			if results[i].Median == 0 {
				return false
			}
			if results[j].Median == 0 {
				return true
			}
			return results[i].Median < results[j].Median
		}
		return results[i].Name < results[j].Name
	})
	return results
}

func resultsByProtocol(results []Result, protocol string) []Result {
	var out []Result
	for _, result := range results {
		if result.Protocol == protocol {
			out = append(out, result)
		}
	}
	return ranked(out)
}

func printProgress(result Result) {
	status := "FAIL"
	if result.OK > 0 {
		status = fmt.Sprintf("OK median=%s ok=%d fail=%d", ms(result.Median), result.OK, result.Fail)
	}
	fmt.Printf("  %-3s %-24s %s\n", result.Protocol, result.Name, status)
}

func printResults(protocol string, results []Result, top int) {
	fmt.Printf("\nTop %d %s resolvers\n", top, protocol)
	fmt.Println(strings.Repeat("-", 104))
	fmt.Printf("%-4s %-24s %-9s %-9s %-9s %-9s %-7s %s\n", "#", "Name", "Median", "Average", "Min", "Max", "OK", "Endpoint")
	fmt.Println(strings.Repeat("-", 104))

	limit := int(math.Min(float64(top), float64(len(results))))
	for i := 0; i < limit; i++ {
		r := results[i]
		fmt.Printf("%-4d %-24s %-9s %-9s %-9s %-9s %-7s %s\n",
			i+1, r.Name, ms(r.Median), ms(r.Average), ms(r.Min), ms(r.Max),
			fmt.Sprintf("%d/%d", r.OK, r.OK+r.Fail), r.Endpoint)
	}
}

func ms(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000)
}

func compactErr(err error) string {
	text := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(text) > 80 {
		return text[:77] + "..."
	}
	return text
}

func printErr(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
}

func pauseBeforeExit(cfg Config) {
	if !cfg.Pause {
		return
	}
	fmt.Print("\nPress Enter to exit...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}
