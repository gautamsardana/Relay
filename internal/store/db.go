package store

import (
	"database/sql"
	_ "github.com/lib/pq"

	"github.com/gautamsardana/relay/internal/config"
	"github.com/gautamsardana/relay/internal/store/sqlc"
)

type Store struct {
	Conn *sql.DB
	queries *sqlc.Queries
}

func New(config *config.Config) (*Store, error){
	db, err := sql.Open("postgres", config.Env.DatabaseURL)
	if err != nil{
		return nil, err
	}
	return &Store{Conn: db, queries: sqlc.New(db)}, nil
}

