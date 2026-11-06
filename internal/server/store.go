package server

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ Pool *pgxpool.Pool }

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

const profileColumns = `id,full_name,birth_date,address,apartment,entrance,floor,access_notes,contact_name,contact_phone,conditions,allergies,updated_at`
const alertColumns = `id,source,status,profile_snapshot,location,created_at,updated_at,deadline_at,received_at,acknowledged_at,closed_at`

func readProfile(ctx context.Context, q queryer) (Profile, error) {
	var p Profile
	err := q.QueryRow(ctx, `SELECT `+profileColumns+` FROM profiles WHERE id='demo-patient'`).Scan(
		&p.ID, &p.FullName, &p.BirthDate, &p.Address, &p.Apartment, &p.Entrance, &p.Floor,
		&p.AccessNotes, &p.ContactName, &p.ContactPhone, &p.Conditions, &p.Allergies, &p.UpdatedAt)
	return p, err
}

func (s *Store) UpdateProfile(ctx context.Context, p Profile) (Profile, error) {
	_, err := s.Pool.Exec(ctx, `UPDATE profiles SET full_name=$1,birth_date=$2,address=$3,apartment=$4,entrance=$5,floor=$6,access_notes=$7,contact_name=$8,contact_phone=$9,conditions=$10,allergies=$11,updated_at=now() WHERE id='demo-patient'`,
		p.FullName, p.BirthDate, p.Address, p.Apartment, p.Entrance, p.Floor, p.AccessNotes, p.ContactName, p.ContactPhone, p.Conditions, p.Allergies)
	if err != nil {
		return Profile{}, err
	}
	return readProfile(ctx, s.Pool)
}

func scanAlert(row pgx.Row) (Alert, error) {
	a := Alert{Events: []Event{}}
	var profileJSON, locationJSON []byte
	err := row.Scan(&a.ID, &a.Source, &a.Status, &profileJSON, &locationJSON, &a.CreatedAt, &a.UpdatedAt, &a.DeadlineAt, &a.ReceivedAt, &a.AcknowledgedAt, &a.ClosedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	if err = json.Unmarshal(profileJSON, &a.Profile); err != nil {
		return a, err
	}
	if len(locationJSON) > 0 && string(locationJSON) != "null" {
		var l Location
		if err = json.Unmarshal(locationJSON, &l); err != nil {
			return a, err
		}
		a.Location = &l
	}
	return a, nil
}

func (s *Store) Dashboard(ctx context.Context, role string) (Dashboard, error) {
	d := Dashboard{Alerts: []Alert{}, Mode: "training"}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return d, err
	}
	defer tx.Rollback(ctx)
	if role == "user" {
		p, err := readProfile(ctx, tx)
		if err != nil {
			return d, err
		}
		d.Profile = &p
	}
	// The cursor and records come from the same snapshot, so reconnecting SSE
	// cannot skip a change committed between two independent reads.
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(id),0),now() FROM alert_events`).Scan(&d.Cursor, &d.ServerTime); err != nil {
		return d, err
	}
	rows, err := tx.Query(ctx, `SELECT `+alertColumns+` FROM alerts WHERE patient_id='demo-patient' ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		a, scanErr := scanAlert(rows)
		if scanErr != nil {
			rows.Close()
			return d, scanErr
		}
		d.Alerts = append(d.Alerts, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	ids := make([]string, len(d.Alerts))
	index := make(map[string]int)
	for i, a := range d.Alerts {
		ids[i] = a.ID
		index[a.ID] = i
	}
	if len(ids) > 0 {
		erows, err := tx.Query(ctx, `SELECT id,alert_id,kind,status,actor,created_at FROM alert_events WHERE alert_id=ANY($1) ORDER BY id`, ids)
		if err != nil {
			return d, err
		}
		for erows.Next() {
			var e Event
			if err = erows.Scan(&e.ID, &e.AlertID, &e.Kind, &e.Status, &e.Actor, &e.CreatedAt); err != nil {
				erows.Close()
				return d, err
			}
			i := index[e.AlertID]
			d.Alerts[i].Events = append(d.Alerts[i].Events, e)
		}
		err = erows.Err()
		erows.Close()
		if err != nil {
			return d, err
		}
	}
	return d, tx.Commit(ctx)
}

func addEvent(ctx context.Context, tx pgx.Tx, id, kind, status, actor string) error {
	_, err := tx.Exec(ctx, `INSERT INTO alert_events(alert_id,patient_id,kind,status,actor) VALUES($1,'demo-patient',$2,$3,$4)`, id, kind, status, actor)
	return err
}

func (s *Store) CreateAlert(ctx context.Context, req CreateRequest) (Alert, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Alert{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize creation for this patient; SQL also enforces one active alert.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('demo-patient',0))`); err != nil {
		return Alert{}, err
	}
	a, err := scanAlert(tx.QueryRow(ctx, `SELECT `+alertColumns+` FROM alerts WHERE id=(SELECT alert_id FROM alert_requests WHERE patient_id='demo-patient' AND client_request_id=$1)`, req.ClientRequestID))
	if err == nil {
		return a, tx.Commit(ctx)
	}
	if !errors.Is(err, ErrNotFound) {
		return Alert{}, err
	}
	a, err = scanAlert(tx.QueryRow(ctx, `SELECT `+alertColumns+` FROM alerts WHERE patient_id='demo-patient' AND status IN ('awaiting_response','received','acknowledged') FOR UPDATE`))
	if err == nil {
		// A manual SOS always bypasses a running simulated response window.
		if req.Source == "manual" && a.Status == "awaiting_response" {
			a, err = scanAlert(tx.QueryRow(ctx, `UPDATE alerts SET status='received',deadline_at=NULL,received_at=now(),updated_at=now() WHERE id=$1 RETURNING `+alertColumns, a.ID))
			if err != nil {
				return Alert{}, err
			}
			if err = addEvent(ctx, tx, a.ID, "manual_override", a.Status, "user"); err != nil {
				return Alert{}, err
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO alert_requests(patient_id,client_request_id,alert_id) VALUES('demo-patient',$1,$2)`, req.ClientRequestID, a.ID); err != nil {
			return Alert{}, err
		}
		return a, tx.Commit(ctx)
	}
	if !errors.Is(err, ErrNotFound) {
		return Alert{}, err
	}
	p, err := readProfile(ctx, tx)
	if err != nil {
		return Alert{}, err
	}
	profileJSON, err := json.Marshal(p)
	if err != nil {
		return Alert{}, err
	}
	var locationJSON []byte
	if req.Location != nil {
		locationJSON, err = json.Marshal(req.Location)
		if err != nil {
			return Alert{}, err
		}
	}
	status := "received"
	var deadline, receivedAt *time.Time
	var dbNow time.Time
	if err = tx.QueryRow(ctx, `SELECT now()`).Scan(&dbNow); err != nil {
		return Alert{}, err
	}
	if req.Source == "simulation" {
		status = "awaiting_response"
		t := dbNow.Add(time.Duration(req.WaitSeconds) * time.Second)
		deadline = &t
	} else {
		receivedAt = &dbNow
	}
	a, err = scanAlert(tx.QueryRow(ctx, `INSERT INTO alerts(id,patient_id,client_request_id,source,status,profile_snapshot,location,deadline_at,received_at) VALUES($1,'demo-patient',$2,$3,$4,$5,$6,$7,$8) RETURNING `+alertColumns,
		newID(), req.ClientRequestID, req.Source, status, profileJSON, locationJSON, deadline, receivedAt))
	if err != nil {
		return Alert{}, err
	}
	if err = addEvent(ctx, tx, a.ID, "created", a.Status, "user"); err != nil {
		return Alert{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO alert_requests(patient_id,client_request_id,alert_id) VALUES('demo-patient',$1,$2)`, req.ClientRequestID, a.ID); err != nil {
		return Alert{}, err
	}
	return a, tx.Commit(ctx)
}

func (s *Store) Transition(ctx context.Context, id, action, role string) (Alert, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Alert{}, err
	}
	defer tx.Rollback(ctx)
	a, err := scanAlert(tx.QueryRow(ctx, `SELECT `+alertColumns+` FROM alerts WHERE id=$1 AND patient_id='demo-patient' FOR UPDATE`, id))
	if err != nil {
		return Alert{}, err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return Alert{}, err
	}
	status, kind := "", ""
	switch {
	case action == "cancel" && role == "user" && (a.Status == "received" || (a.Status == "awaiting_response" && a.DeadlineAt != nil && now.Before(*a.DeadlineAt))):
		status, kind = "cancelled", "cancelled"
	case action == "acknowledge" && role == "dispatcher" && a.Status == "received":
		status, kind = "acknowledged", "acknowledged"
	case action == "close" && role == "dispatcher" && a.Status == "acknowledged":
		status, kind = "closed", "closed"
	default:
		return Alert{}, ErrConflict
	}
	a, err = scanAlert(tx.QueryRow(ctx, `UPDATE alerts SET status=$2,updated_at=clock_timestamp(),deadline_at=NULL,acknowledged_at=CASE WHEN $2='acknowledged' THEN clock_timestamp() ELSE acknowledged_at END,closed_at=CASE WHEN $2 IN ('closed','cancelled') THEN clock_timestamp() ELSE closed_at END WHERE id=$1 RETURNING `+alertColumns, id, status))
	if err != nil {
		return Alert{}, err
	}
	if err = addEvent(ctx, tx, id, kind, status, role); err != nil {
		return Alert{}, err
	}
	return a, tx.Commit(ctx)
}

func (s *Store) ProcessDue(ctx context.Context) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id FROM alerts WHERE status='awaiting_response' AND deadline_at<=now() FOR UPDATE SKIP LOCKED LIMIT 50`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.Exec(ctx, `UPDATE alerts SET status='received',received_at=now(),updated_at=now(),deadline_at=NULL WHERE id=$1`, id); err != nil {
			return err
		}
		if err = addEvent(ctx, tx, id, "response_timeout", "received", "system"); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE expires_at<now()`)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) EventsAfter(ctx context.Context, cursor int64) ([]Event, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,alert_id,kind,status,actor,created_at FROM alert_events WHERE id>$1 AND patient_id='demo-patient' ORDER BY id LIMIT 100`, cursor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var e Event
		if err = rows.Scan(&e.ID, &e.AlertID, &e.Kind, &e.Status, &e.Actor, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
