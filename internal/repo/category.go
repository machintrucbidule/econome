package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"econome/internal/domain"
)

type categoryRepo struct{}

const selectCategory = `SELECT id, user_id, name, parent_id, flow_type, default_expanded, status,
	created_at, updated_at FROM category`

func (categoryRepo) Create(ctx context.Context, q DBTX, c *domain.Category) (int64, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO category (user_id, name, parent_id, flow_type, default_expanded, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.UserID, c.Name, nullInt64(c.ParentID), string(c.FlowType), boolToInt(c.DefaultExpanded),
		nullStatus(c.Status), formatTime(c.CreatedAt), formatTime(c.UpdatedAt))
	if err != nil {
		return 0, fmt.Errorf("repo: create category: %w", err)
	}
	return res.LastInsertId()
}

func (categoryRepo) Get(ctx context.Context, q DBTX, userID, id int64) (*domain.Category, error) {
	return scanCategory(q.QueryRowContext(ctx, selectCategory+` WHERE user_id = ? AND id = ?`, userID, id))
}

func (categoryRepo) Update(ctx context.Context, q DBTX, c *domain.Category) error {
	_, err := q.ExecContext(ctx,
		`UPDATE category SET name = ?, parent_id = ?, flow_type = ?, default_expanded = ?, status = ?, updated_at = ?
		 WHERE user_id = ? AND id = ?`,
		c.Name, nullInt64(c.ParentID), string(c.FlowType), boolToInt(c.DefaultExpanded), nullStatus(c.Status),
		formatTime(nowUTC()), c.UserID, c.ID)
	if err != nil {
		return fmt.Errorf("repo: update category: %w", err)
	}
	return nil
}

func (categoryRepo) Delete(ctx context.Context, q DBTX, userID, id int64) error {
	res, err := q.ExecContext(ctx, `DELETE FROM category WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return domain.ErrConflict
		}
		return fmt.Errorf("repo: delete category: %w", err)
	}
	return notFoundIfNoRows(res)
}

func (categoryRepo) ListByUser(ctx context.Context, q DBTX, userID int64) ([]domain.Category, error) {
	rows, err := q.QueryContext(ctx, selectCategory+` WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("repo: list categories: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Category
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func scanCategory(row rowScanner) (*domain.Category, error) {
	var (
		c                    domain.Category
		parentID             sql.NullInt64
		defaultExpanded      int
		createdAt, updatedAt string
	)
	err := row.Scan(&c.ID, &c.UserID, &c.Name, &parentID, &c.FlowType, &defaultExpanded, &c.Status, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: scan category: %w", err)
	}
	c.ParentID = ptrInt64(parentID)
	c.DefaultExpanded = defaultExpanded != 0
	if c.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, err
	}
	if c.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}
