package cache

import (
	"context"

	"github.com/redis/go-redis/v9"
)

const productListCachePattern = "products:list:*"

type ProductCache struct {
	client *redis.Client
}

func NewProductCache(client *redis.Client) *ProductCache {
	return &ProductCache{client: client}
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
	return c.client.Del(ctx, "products:detail:"+productID).Err()
}
