package store

import (
	"database/sql"

	"github.com/gautamsardana/relay/internal/config"
)

type Store struct {
	Conn *sql.DB
}

func New(config *config.Config) (*Store, error){
	db, err := sql.Open("postgres", config.Env.DatabaseURL)
	if err != nil{
		return nil, err
	}
	return &Store{Conn: db}, nil
}

