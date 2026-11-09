package modelmetadata

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// TestCatalogPayloadSniffsZstdAndPlainJSON covers OMP's precompressed endpoint
// and mirrors that send JSON regardless of URL or Content-Type. Truncated frames
// must return an error so refresh preserves the previous usable snapshot.
func TestCatalogPayloadSniffsZstdAndPlainJSON(t *testing.T) {
	plain := []byte(`{"fixture":{"models":{"one":{"name":"One","int":42,"tps":123}}}}`)
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	compressed := encoder.EncodeAll(plain, nil)
	for _, body := range [][]byte{plain, compressed} {
		got, err := readCatalogPayload(bytes.NewReader(body))
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("payload=%q error=%v", got, err)
		}
	}
	if _, err := readCatalogPayload(bytes.NewReader(compressed[:len(compressed)-2])); err == nil {
		t.Fatal("accepted truncated zstd frame")
	}
}

// TestRefreshReadsPrecompressedCatalog verifies the real HTTP refresh boundary,
// not merely the decoder: successful decompression must publish parsed scores.
func TestRefreshReadsPrecompressedCatalog(t *testing.T) {
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	payload := encoder.EncodeAll([]byte(`{"fixture":{"models":{"one":{"name":"One","int":42,"tps":123}}}}`), nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	service := New(Options{URL: server.URL})
	allowNetwork := true
	result, err := service.Refresh(context.Background(), RefreshOptions{AllowNetwork: &allowNetwork, Force: true})
	if err != nil || result.Error != "" || len(result.Rows) != 1 {
		t.Fatalf("refresh rows=%d error=%v %s", len(result.Rows), err, result.Error)
	}
	if result.Rows[0].Int == nil || *result.Rows[0].Int != 42 || result.Rows[0].TPS == nil || *result.Rows[0].TPS != 123 {
		t.Fatal(result.Rows[0])
	}
}
