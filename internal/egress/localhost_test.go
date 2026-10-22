package egress

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestLocalhostDirectIPv4AndIPv6(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "[::1]:0"} {
		t.Run(address, func(t *testing.T) {
			listener, err := net.Listen("tcp", address)
			if err != nil {
				if strings.HasPrefix(address, "[") {
					t.Skipf("IPv6 loopback unavailable: %v", err)
				}
				t.Fatal(err)
			}
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			host := "LOCALHOST:" + port
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Host != host || r.URL.Path != "/v1/models" {
					t.Errorf("request changed: Host=%q path=%q", r.Host, r.URL.Path)
				}
				io.WriteString(w, "local")
			}))
			server.Listener.Close()
			server.Listener = listener
			server.Start()
			defer server.Close()
			m, _ := testManager(t)
			resp := get(t, m, "http://"+host+"/v1/models")
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || string(body) != "local" {
				t.Fatalf("local response: %q %v", body, err)
			}
		})
	}
}

func TestLocalhostDialUsesOnlyLoopbackAndHonorsCancellation(t *testing.T) {
	var addresses []string
	failure := errors.New("synthetic dial failure")
	dial := localhostDialer(func(ctx context.Context, network, address string) (net.Conn, error) {
		addresses = append(addresses, address)
		return nil, failure
	})
	_, err := dial(context.Background(), "tcp", "localhost:1234")
	if !errors.Is(err, failure) || !reflect.DeepEqual(addresses, []string{"127.0.0.1:1234", "[::1]:1234"}) {
		t.Fatalf("dial attempts: %v %v", addresses, err)
	}
	addresses = nil
	_, _ = dial(context.Background(), "tcp", "localhost.example:1234")
	if !reflect.DeepEqual(addresses, []string{"localhost.example:1234"}) {
		t.Fatal("non-localhost dial changed", addresses)
	}
	ctx, cancel := context.WithCancel(context.Background())
	addresses = nil
	dial = localhostDialer(func(ctx context.Context, network, address string) (net.Conn, error) {
		addresses = append(addresses, address)
		cancel()
		return nil, ctx.Err()
	})
	if _, err = dial(ctx, "tcp", "localhost:1234"); !errors.Is(err, context.Canceled) || len(addresses) != 1 {
		t.Fatalf("cancellation retried: %v %v", addresses, err)
	}
}

func TestLocalhostKeepsExplicitProxyRoute(t *testing.T) {
	m, st := testManager(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "localhost:1" || !r.URL.IsAbs() {
			t.Errorf("proxy target changed: %s", r.URL)
		}
		io.WriteString(w, "proxy")
	}))
	defer proxy.Close()
	put(t, st, "http", proxy.URL)
	bind(t, st, "", "proxy")
	resp := get(t, m, "http://localhost:1/v1/models")
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(body) != "proxy" {
		t.Fatalf("proxy route: %q %v", body, err)
	}
	proxy.Close()
	request, _ := http.NewRequestWithContext(WithRoute(context.Background(), "p", "default"), "GET", "http://localhost:1/v1/models", nil)
	if _, err = m.RoundTrip(request); !errors.Is(err, ErrUnavailable) {
		t.Fatal("proxy failure must remain unavailable", err)
	}
}
