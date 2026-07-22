package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/RedditUclaista/chat-service/internal/entities"
	"github.com/gocql/gocql"
	"github.com/valkey-io/valkey-go"
)

type valkeyCache struct {
	client valkey.Client
}

func NewValkeyCache(client valkey.Client) ChatCache {
	return &valkeyCache{client: client}
}

func roomMembersKey(roomID gocql.UUID) string {
	return fmt.Sprintf("room:%s:members", roomID.String())
}

func recentMessagesKey(roomID gocql.UUID) string {
	return fmt.Sprintf("room:%s:messages:recent", roomID.String())
}

func unreadKey(userID, roomID gocql.UUID) string {
	return fmt.Sprintf("unread:%s:%s", userID.String(), roomID.String())
}

func (c *valkeyCache) IsMember(ctx context.Context, roomID, userID gocql.UUID) (bool, error) {
	key := roomMembersKey(roomID)
	resp := c.client.Do(ctx, c.client.B().Sismember().Key(key).Member(userID.String()).Build())
	return resp.ToBool()
}

func (c *valkeyCache) SetRoomMembers(ctx context.Context, roomID gocql.UUID, memberIDs []gocql.UUID, ttl time.Duration) error {
	key := roomMembersKey(roomID)

	members := make([]string, 0, len(memberIDs))
	for _, uid := range memberIDs {
		members = append(members, uid.String())
	}

	if err := c.client.Do(ctx, c.client.B().Sadd().Key(key).Member(members...).Build()).Error(); err != nil {
		return fmt.Errorf("set room members: %w", err)
	}

	c.client.Do(ctx, c.client.B().Expire().Key(key).Seconds(int64(ttl.Seconds())).Build())
	return nil
}

func (c *valkeyCache) AddRecentMessage(ctx context.Context, msg entities.Message) error {
	key := recentMessagesKey(msg.RoomID)

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	if err := c.client.Do(ctx, c.client.B().Lpush().Key(key).Element(string(data)).Build()).Error(); err != nil {
		return fmt.Errorf("add recent message: %w", err)
	}

	c.client.Do(ctx, c.client.B().Ltrim().Key(key).Start(0).Stop(99).Build())
	return nil
}

func (c *valkeyCache) GetRecentMessages(ctx context.Context, roomID gocql.UUID, limit int) ([]entities.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	key := recentMessagesKey(roomID)
	resp := c.client.Do(ctx, c.client.B().Lrange().Key(key).Start(0).Stop(int64(limit-1)).Build())

	vals, err := resp.AsStrSlice()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get recent messages: %w", err)
	}

	messages := make([]entities.Message, 0, len(vals))
	for _, v := range vals {
		var msg entities.Message
		if err := json.Unmarshal([]byte(v), &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (c *valkeyCache) GetUnreadCount(ctx context.Context, userID, roomID gocql.UUID) (int, error) {
	key := unreadKey(userID, roomID)
	resp := c.client.Do(ctx, c.client.B().Get().Key(key).Build())

	val, err := resp.ToInt64()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("get unread count: %w", err)
	}
	return int(val), nil
}

func (c *valkeyCache) IncrementUnreadCount(ctx context.Context, userID, roomID gocql.UUID) error {
	key := unreadKey(userID, roomID)
	return c.client.Do(ctx, c.client.B().Incr().Key(key).Build()).Error()
}

func (c *valkeyCache) ResetUnreadCount(ctx context.Context, userID, roomID gocql.UUID) error {
	key := unreadKey(userID, roomID)
	return c.client.Do(ctx, c.client.B().Del().Key(key).Build()).Error()
}

func (c *valkeyCache) Close() error {
	c.client.Close()
	return nil
}
