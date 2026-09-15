package repository

import (
	"context"
	"fmt"

	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository/dbsqlc"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SQLCUserStore implements UserPersistence using sqlc-generated queries over pgx.
type SQLCUserStore struct {
	q *dbsqlc.Queries
}

// NewSQLCUserStore returns a UserPersistence backed by pool.
func NewSQLCUserStore(pool *pgxpool.Pool) *SQLCUserStore {
	return &SQLCUserStore{q: dbsqlc.New(pool)}
}

func userFromDB(u dbsqlc.User) models.User {
	return models.User{
		ID:        uint(u.ID),
		Username:  u.Username,
		Password:  u.Password,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

func (s *SQLCUserStore) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	row, err := s.q.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, mapNotFound(err)
	}
	u := userFromDB(row)
	return &u, nil
}

func (s *SQLCUserStore) FindByID(ctx context.Context, id uint) (*models.User, error) {
	row, err := s.q.GetUserByID(ctx, toInt64(id))
	if err != nil {
		return nil, mapNotFound(err)
	}
	u := userFromDB(row)
	return &u, nil
}

func (s *SQLCUserStore) Create(ctx context.Context, user *models.User) error {
	row, err := s.q.CreateUser(ctx, dbsqlc.CreateUserParams{
		Username: user.Username,
		Password: user.Password,
		Role:     user.Role,
	})
	if err != nil {
		if isUsernameUniqueConstraintError(err) {
			return fmt.Errorf("%w: %v", ErrUserUsernameConflict, err)
		}
		return err
	}
	*user = userFromDB(row)
	return nil
}

var _ UserPersistence = (*SQLCUserStore)(nil)
