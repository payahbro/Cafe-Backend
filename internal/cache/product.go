package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"cafeTelkom/internal/service"

	"github.com/redis/go-redis/v9"
)

const productListCachePattern = "products:list:*"
const productListCacheTTL = 5 * time.Minute
const productDetailCacheTTL = 10 * time.Minute

type ProductCache struct {
	client *redis.Client
}

func NewProductCache(client *redis.Client) *ProductCache {
	return &ProductCache{client: client}
}

func (c *ProductCache) GetProductList(ctx context.Context, input service.ListProductsInput) (*service.ProductList, error) {
	if c == nil || c.client == nil {
		return nil, service.ErrProductCacheMiss
	}

	value, err := c.client.Get(ctx, productListCacheKey(input)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrProductCacheMiss
		}
		return nil, err
	}

	var list service.ProductList
	if err := json.Unmarshal(value, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

func (c *ProductCache) SetProductList(ctx context.Context, input service.ListProductsInput, list service.ProductList) error {
	if c == nil || c.client == nil {
		return nil
	}

	value, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, productListCacheKey(input), value, productListCacheTTL).Err()
}

func (c *ProductCache) GetProductDetail(ctx context.Context, productID string) (*service.Product, error) {
	if c == nil || c.client == nil {
		return nil, service.ErrProductCacheMiss
	}

	value, err := c.client.Get(ctx, productDetailCacheKey(productID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrProductCacheMiss
		}
		return nil, err
	}

	var product service.Product
	if err := json.Unmarshal(value, &product); err != nil {
		return nil, err
	}
	return &product, nil
}

func (c *ProductCache) SetProductDetail(ctx context.Context, product service.Product) error {
	if c == nil || c.client == nil {
		return nil
	}

	value, err := json.Marshal(product)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, productDetailCacheKey(product.ID), value, productDetailCacheTTL).Err()
}

func (c *ProductCache) InvalidateProductLists(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}

	iter := c.client.Scan(ctx, 0, productListCachePattern, 100).Iterator()
	keys := make([]string, 0, 100)
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) == cap(keys) {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
			keys = keys[:0]
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

func (c *ProductCache) InvalidateProductDetail(ctx context.Context, productID string) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Del(ctx, productDetailCacheKey(productID)).Err()
}

func productListCacheKey(input service.ListProductsInput) string {
	raw := fmt.Sprintf("limit=%d:offset=%d:include_deleted=%t", input.Limit, input.Offset, input.IncludeDeleted)
	hash := sha256.Sum256([]byte(raw))
	return "products:list:" + hex.EncodeToString(hash[:])
}

func productDetailCacheKey(productID string) string {
	return "products:detail:" + productID
}
