package client

import (
	"context"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/docreader/proto"
	ipclass "github.com/Tencent/WeKnora/internal/ipclass"
	"google.golang.org/grpc/resolver"
)

func TestNewClientDoesNotChangeGlobalGRPCResolver(t *testing.T) {
	previous := resolver.GetDefaultScheme()
	resolver.SetDefaultScheme("passthrough")
	t.Cleanup(func() { resolver.SetDefaultScheme(previous) })

	client, err := NewClientWithAuth("127.0.0.1:1", &AuthConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if got := resolver.GetDefaultScheme(); got != "passthrough" {
		t.Fatalf("DocReader changed global gRPC resolver to %q", got)
	}
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	log.Println("INFO: Initializing DocReader client tests")
}

func requireLiveDocReaderClient(t *testing.T) *Client {
	t.Helper()

	client, err := NewClient("localhost:50051")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := client.ListEngines(ctx, &proto.ListEnginesRequest{}); err != nil {
		t.Skipf("DocReader gRPC server not available at localhost:50051: %v", err)
	}

	return client
}

// skipScrapeUnlessDNSPublic skips live scrape tests when the network's DNS
// yields a restricted address for the public test host. The SSRF guard
// rejects such URLs on purpose, and even whitelisted the address would not
// be reachable, so the scrape cannot be verified without a real public route.
func skipScrapeUnlessDNSPublic(t *testing.T, host string) {
	t.Helper()
	addrs, err := net.LookupIP(host)
	if err != nil || len(addrs) == 0 {
		t.Skipf("cannot resolve %q in this environment: %v", host, err)
	}
	for _, ip := range addrs {
		class, _ := ipclass.Classify(ip)
		if class == ipclass.Public {
			return
		}
	}
	t.Skipf("DNS on this network resolves %q into a restricted range; the public scrape cannot be verified here", host)
}

func TestReadURL(t *testing.T) {
	skipScrapeUnlessDNSPublic(t, "example.com")
	client := requireLiveDocReaderClient(t)
	client.SetDebug(true)

	startTime := time.Now()
	resp, err := client.Read(
		context.Background(),
		&proto.ReadRequest{
			Url:   "https://example.com",
			Title: "test",
		},
	)
	log.Printf("INFO: Read(URL) completed in %v", time.Since(startTime))

	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("Read returned error: %s", resp.Error)
	}
	if resp.MarkdownContent == "" {
		t.Error("Expected non-empty markdown content")
	}
	log.Printf("INFO: content_len=%d, images=%d", len(resp.MarkdownContent), len(resp.ImageRefs))
}

func TestReadFile(t *testing.T) {
	client := requireLiveDocReaderClient(t)
	client.SetDebug(true)

	fileContent, err := os.ReadFile("../testdata/test.md")
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	startTime := time.Now()
	resp, err := client.Read(
		context.Background(),
		&proto.ReadRequest{
			FileContent: fileContent,
			FileName:    "test.md",
			FileType:    "md",
		},
	)
	log.Printf("INFO: Read(file) completed in %v", time.Since(startTime))

	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("Read returned error: %s", resp.Error)
	}
	if resp.MarkdownContent == "" {
		t.Error("Expected non-empty markdown content")
	}

	imageRefs := GetImageRefsFromResponse(resp)
	log.Printf("INFO: content_len=%d, images=%d", len(resp.MarkdownContent), len(imageRefs))
}
