package cache

import (
	"testing"

	"cafeTelkom/internal/service"
)

func TestProductListCacheKeyIncludesDeletedFilter(t *testing.T) {
	publicKey := productListCacheKey(service.ListProductsInput{Limit: 10})
	includeDeletedKey := productListCacheKey(service.ListProductsInput{Limit: 10, IncludeDeleted: true})

	if publicKey == includeDeletedKey {
		t.Fatalf("expected different cache keys for include deleted filter")
	}
}
