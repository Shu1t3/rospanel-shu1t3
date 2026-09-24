package store

import (
	"database/sql"
	"errors"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// RegistrationUser contains credentials prepared before the approval transaction.
type RegistrationUser struct {
	Name, UUID, Password, SubToken string
	Plan                           *UserPlanWrite
}

// ApproveRegistrationRequest commits the request decision, account, plan, and chat
// link together. A crash before commit leaves the request retryable.
func (s *Store) ApproveRegistrationRequest(id int64, in RegistrationUser) (*model.User, bool, bool, error) {
	var userID int64
	var claimed bool
	var alreadyLinked bool
	err := s.withTx(func(tx *sql.Tx) error {
		var chatID int64
		if err := tx.QueryRow(`SELECT chat_id FROM registration_requests WHERE id = ?`, id).Scan(&chatID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		claimed = true
		// The chat may have been linked through a panel code since the request was made.
		var existingID int64
		err := tx.QueryRow(`SELECT id FROM users WHERE tg_chat_id = ? AND tg_chat_id <> 0 LIMIT 1`, chatID).Scan(&existingID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			userID = existingID
			alreadyLinked = true
		} else {
			err = tx.QueryRow(`INSERT INTO users (name, uuid, password, sub_token, tg_chat_id)
				VALUES (?, ?, ?, ?, ?) RETURNING id`, in.Name, in.UUID, encField(in.Password), in.SubToken, chatID).Scan(&userID)
			if err != nil {
				return err
			}
			if in.Plan != nil {
				plan := *in.Plan
				plan.UserID = userID
				if err := applyUserPlanOn(tx, plan); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(dropPrevTelegramChatSQL, chatID); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`DELETE FROM registration_requests WHERE id = ?`, id)
		return err
	})
	if err != nil || !claimed {
		return nil, claimed, false, err
	}
	u, err := s.GetUser(userID)
	return u, true, alreadyLinked, err
}

// Moderated self-registration requests. A request is a signup held for an admin
// decision; approving it is what actually creates the user (see core.Manager).

// CreateRegistrationRequest inserts a pending request for a chat. chat_id is unique,
// so a chat with an existing request keeps the first one (ErrRegistrationPending).
func (s *Store) CreateRegistrationRequest(chatID int64, name string, now int64) (*model.RegistrationRequest, error) {
	res, err := s.db.Exec(
		`INSERT INTO registration_requests (chat_id, name, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(chat_id) DO NOTHING`,
		chatID, name, now)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrRegistrationPending
	}
	return s.GetRegistrationRequestByChat(chatID)
}

// ErrRegistrationPending means the chat already has a pending request.
var ErrRegistrationPending = errors.New("registration already pending")

// GetRegistrationRequest returns one request by id.
func (s *Store) GetRegistrationRequest(id int64) (*model.RegistrationRequest, error) {
	return s.scanRegistrationRequest(`WHERE id = ?`, id)
}

// GetRegistrationRequestByChat returns a chat's pending request, or nil when none.
func (s *Store) GetRegistrationRequestByChat(chatID int64) (*model.RegistrationRequest, error) {
	r, err := s.scanRegistrationRequest(`WHERE chat_id = ?`, chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

func (s *Store) scanRegistrationRequest(where string, args ...any) (*model.RegistrationRequest, error) {
	var r model.RegistrationRequest
	err := s.db.QueryRow(
		`SELECT id, chat_id, name, created_at FROM registration_requests `+where, args...).
		Scan(&r.ID, &r.ChatID, &r.Name, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRegistrationRequests returns all pending requests, oldest first.
func (s *Store) ListRegistrationRequests() ([]model.RegistrationRequest, error) {
	rows, err := s.db.Query(`SELECT id, chat_id, name, created_at FROM registration_requests ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.RegistrationRequest
	for rows.Next() {
		var r model.RegistrationRequest
		if err := rows.Scan(&r.ID, &r.ChatID, &r.Name, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRegistrationRequest removes a request (after approval or rejection).
func (s *Store) DeleteRegistrationRequest(id int64) error {
	_, err := s.db.Exec(`DELETE FROM registration_requests WHERE id = ?`, id)
	return err
}

// ClaimRegistrationRequest atomically deletes a request and reports whether THIS
// call removed it. With MaxOpenConns(1) serialising writes, exactly one of several
// concurrent approvers/rejecters wins — so a double-click (or two admins) can't
// create the account twice or send contradictory notifications.
func (s *Store) ClaimRegistrationRequest(id int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM registration_requests WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
