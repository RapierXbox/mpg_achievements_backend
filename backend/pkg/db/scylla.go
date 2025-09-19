package db

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gocql/gocql"
)

// init scylladb connection
func InitScyllaDB(host string, logger *log.Logger) (*gocql.Session, error) {
	cluster := gocql.NewCluster(host)
	cluster.Consistency = gocql.One // only a single node needs to respond for now (maby change to Quorum)
	cluster.Timeout = 5 * time.Second
	cluster.ConnectTimeout = 5 * time.Second
	cluster.NumConns = 2 // two parralel tcp connections
	cluster.ReconnectInterval = 1 * time.Second
	cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(gocql.RoundRobinHostPolicy()) // for a single node not really relevant

	// enable compression
	cluster.Compressor = gocql.SnappyCompressor{}

	// create session with connection pool
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}

	// verify connection with system query
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := session.Query("SELECT now() FROM system.local").WithContext(ctx).Exec(); err != nil {
		return nil, fmt.Errorf("system check failed: %w", err)
	}

	logger.Printf("connected to scylladb cluster at %s", host)
	return session, nil
}

// execute database schema migrations
func RunMigrations(session *gocql.Session, logger *log.Logger) error {
	// create keyspaces if not exists
	createKeyspace := `CREATE KEYSPACE IF NOT EXISTS auth WITH replication = {
		'class': 'SimpleStrategy',
		'replication_factor': 1
	}`

	if err := session.Query(createKeyspace).Exec(); err != nil {
		return fmt.Errorf("keyspace creation failed: %w", err)
	}

	createKeyspace = `CREATE KEYSPACE IF NOT EXISTS qr WITH replication = {
		'class': 'SimpleStrategy',
		'replication_factor': 1
	}`

	if err := session.Query(createKeyspace).Exec(); err != nil {
		return fmt.Errorf("keyspace creation failed: %w", err)
	}

	// create tables
	tables := []string{
		// accounts table
		`CREATE TABLE IF NOT EXISTS auth.accounts (
			id UUID PRIMARY KEY,
			email TEXT,
			password_hash TEXT,
			created_at TIMESTAMP,
			admin BOOLEAN,
		)`,

		`CREATE INDEX IF NOT EXISTS idx_email ON auth.accounts(email);`,

		// permanent sessions table
		`CREATE TABLE IF NOT EXISTS auth.permanent_sessions (
			user_id UUID,
			device_id UUID,
			token_hash TEXT,
			created_at TIMESTAMP,
			last_used TIMESTAMP,
			expires_at TIMESTAMP,
			PRIMARY KEY (user_id, device_id)
		)`,

		// qr actions table
		`CREATE TABLE IF NOT EXISTS qr.qr_actions (
			id UUID PRIMARY KEY,
			action_json TEXT,
		)`,

		// user qr scans table
		`CREATE TABLE IF NOT EXISTS qr.user_qr_scans (
			user_id UUID,
			qr_code_id UUID,
			count INT,
			PRIMARY KEY (user_id, qr_code_id)
		)`,

		// qr codes table
		`CREATE TABLE IF NOT EXISTS qr.qr_codes (
			id UUID PRIMARY KEY,
			action_id UUID,
			qr_code_type INT,
			max_usages INT,
			expires_at TIMESTAMP
		)`,
	}

	// execute all table creation queries
	for i, query := range tables {
		if err := session.Query(query).Exec(); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	logger.Println("database schema initialized")
	return nil
}
