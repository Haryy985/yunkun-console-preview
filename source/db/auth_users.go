package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type AuthUser struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	Disabled     bool   `json:"disabled"`
	TokenVersion int64  `json:"-"`
}

func scanAuthUser(row interface{ Scan(...any) error }) (*AuthUser, error) {
	var u AuthUser
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Disabled, &u.TokenVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

const authUserColumns = `id,username,password_hash,role,disabled,token_version`

func (d *DB) AuthUserByName(name string) (*AuthUser, error) {
	return scanAuthUser(d.QueryRow(`SELECT `+authUserColumns+` FROM auth_users WHERE username=$1`, name))
}

func (d *DB) AuthUserByID(id int64) (*AuthUser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return scanAuthUser(d.QueryRowContext(ctx, `SELECT `+authUserColumns+` FROM auth_users WHERE id=$1`, id))
}

func (d *DB) ListAuthUsers() ([]AuthUser, error) {
	rows, err := d.Query(`SELECT ` + authUserColumns + ` FROM auth_users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []AuthUser{}
	for rows.Next() {
		var u AuthUser
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Disabled, &u.TokenVersion); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (d *DB) CreateAuthUser(name, hash, role string) (*AuthUser, error) {
	var id int64
	err := d.QueryRow(`INSERT INTO auth_users(username,password_hash,role) VALUES ($1,$2,$3) RETURNING id`,
		name, hash, role).Scan(&id)
	if err != nil {
		return nil, err
	}
	return d.AuthUserByID(id)
}

func (d *DB) UpdateAuthUser(id int64, role string, disabled bool) (*AuthUser, error) {
	res, err := d.Exec(`UPDATE auth_users SET role=$2,disabled=$3,token_version=token_version+1,updated_at=now() WHERE id=$1`,
		id, role, disabled)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	return d.AuthUserByID(id)
}

func (d *DB) SetAuthUserPassword(id int64, hash string) error {
	_, err := d.Exec(`UPDATE auth_users SET password_hash=$2,token_version=token_version+1,updated_at=now() WHERE id=$1`, id, hash)
	return err
}
