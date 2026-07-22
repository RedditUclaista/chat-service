package database

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gocql/gocql"
)

type MigrateConfig struct {
	Host              string
	Keyspace          string
	ReplicationFactor int
}

func RunMigrations(cfg MigrateConfig) error {
	cluster := gocql.NewCluster(cfg.Host)
	cluster.Consistency = gocql.All
	cluster.Timeout = 10 * time.Second
	cluster.ConnectTimeout = 15 * time.Second
	cluster.Port = 9042

	session, err := cluster.CreateSession()
	if err != nil {
		return fmt.Errorf("conectando a system keyspace: %w", err)
	}
	defer session.Close()

	err = session.Query(fmt.Sprintf(
		`CREATE KEYSPACE IF NOT EXISTS %s
		 WITH replication = {'class': 'SimpleStrategy', 'replication_factor': %d}`,
		cfg.Keyspace, cfg.ReplicationFactor,
	)).Exec()
	if err != nil {
		return fmt.Errorf("creando keyspace %s: %w", cfg.Keyspace, err)
	}
	slog.Info("keyspace asegurado", "keyspace", cfg.Keyspace)

	cluster.Keyspace = cfg.Keyspace
	targetSession, err := cluster.CreateSession()
	if err != nil {
		return fmt.Errorf("conectando a keyspace %s: %w", cfg.Keyspace, err)
	}
	defer targetSession.Close()

	tables := []struct{ name, cql string }{
		{"chat_rooms", chatRoomsTableCQL},
		{"chat_room_members", chatRoomMembersTableCQL},
		{"messages_by_room", messagesByRoomTableCQL},
		{"active_chats_by_user", activeChatsByUserTableCQL},
	}

	for _, t := range tables {
		if err := targetSession.Query(t.cql).Exec(); err != nil {
			return fmt.Errorf("creando tabla %s: %w", t.name, err)
		}
		slog.Info("tabla asegurada", "table", t.name)
	}

	slog.Info("migraciones ejecutadas correctamente")
	return nil
}

const chatRoomsTableCQL = `
	CREATE TABLE IF NOT EXISTS chat_rooms (
		room_id UUID,
		room_type TEXT,
		name TEXT,
		created_by UUID,
		created_at TIMESTAMP,
		PRIMARY KEY (room_id)
	)`

const chatRoomMembersTableCQL = `
	CREATE TABLE IF NOT EXISTS chat_room_members (
		room_id UUID,
		user_id UUID,
		role TEXT,
		joined_at TIMESTAMP,
		PRIMARY KEY (room_id, user_id)
	)`

const messagesByRoomTableCQL = `
	CREATE TABLE IF NOT EXISTS messages_by_room (
		room_id UUID,
		message_id TIMEUUID,
		sender_id UUID,
		content_body TEXT,
		is_read BOOLEAN,
		created_at TIMESTAMP,
		PRIMARY KEY (room_id, message_id)
	) WITH CLUSTERING ORDER BY (message_id DESC)`

const activeChatsByUserTableCQL = `
	CREATE TABLE IF NOT EXISTS active_chats_by_user (
		user_id UUID,
		last_activity TIMESTAMP,
		room_id UUID,
		last_message_preview TEXT,
		unread_count INT,
		room_name TEXT,
		room_type TEXT,
		PRIMARY KEY (user_id, last_activity)
	) WITH CLUSTERING ORDER BY (last_activity DESC)`
