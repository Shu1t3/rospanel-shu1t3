package store

import (
	"database/sql"
	"errors"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// createWebRegistrationOn stores the account, its website identity and its plan
// together. A failed plan write cannot leave an account the website cannot reach.
func createWebRegistrationOn(tx *sql.Tx, in RegistrationUser, externalID, source string, refID int64) (int64, error) {
	var id int64
	if err := tx.QueryRow(`INSERT INTO users (name, uuid, password, sub_token, external_id, source)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id`, in.Name, in.UUID, encField(in.Password), in.SubToken, externalID, source).Scan(&id); err != nil {
		return 0, err
	}
	if in.Plan != nil {
		plan := *in.Plan
		plan.UserID = id
		if err := applyUserPlanOn(tx, plan); err != nil {
			return 0, err
		}
	}
	if refID != 0 {
		if _, err := tx.Exec(`UPDATE users SET referrer_id = ? WHERE id = ? AND ? <> id
			AND EXISTS (SELECT 1 FROM users r WHERE r.id = ? AND r.referrer_id <> ?)`, refID, id, refID, refID, id); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// CreateWebRegistration commits all registration data before callers announce it.
func (s *Store) CreateWebRegistration(in RegistrationUser, externalID, source string, refID int64) (*model.User, error) {
	var id int64
	err := s.withTx(func(tx *sql.Tx) error {
		var err error
		id, err = createWebRegistrationOn(tx, in, externalID, source, refID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetUser(id)
}

// ApproveWebRegistration commits the account and the request decision together.
// created is false if another decision won or the website already has an account.
func (s *Store) ApproveWebRegistration(requestID int64, in RegistrationUser) (u *model.User, created bool, err error) {
	var id int64
	err = s.withTx(func(tx *sql.Tx) error {
		var ext, source string
		var refID int64
		err := tx.QueryRow(`SELECT external_id, source, referrer_id FROM registration_requests
			WHERE id = ? AND external_id IS NOT NULL`, requestID).Scan(&ext, &source, &refID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		err = tx.QueryRow(userByExternalIDSQL, ext).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			id, err = createWebRegistrationOn(tx, in, ext, source, refID)
			created = err == nil
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM registration_requests WHERE id = ?`, requestID)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	if id == 0 {
		return nil, false, nil
	}
	u, err = s.GetUser(id)
	return u, created, err
}
