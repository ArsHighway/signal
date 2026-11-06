-- +goose Up
CREATE TABLE profiles (
    id text PRIMARY KEY,
    full_name text NOT NULL,
    birth_date text NOT NULL DEFAULT '',
    address text NOT NULL,
    apartment text NOT NULL DEFAULT '',
    entrance text NOT NULL DEFAULT '',
    floor text NOT NULL DEFAULT '',
    access_notes text NOT NULL DEFAULT '',
    contact_name text NOT NULL DEFAULT '',
    contact_phone text NOT NULL DEFAULT '',
    conditions text NOT NULL DEFAULT '',
    allergies text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO profiles (id, full_name, birth_date, address, apartment, entrance, floor, contact_name, contact_phone, conditions, allergies)
VALUES ('demo-patient', 'Алексей Тестовый', '1958-04-12', 'Учебный город, Демонстрационная улица, дом 1', '12', '2', '3', 'Анна — учебный контакт', '', 'Демонстрационные сведения', 'Не указаны');

CREATE TABLE sessions (
    token_hash text PRIMARY KEY,
    role text NOT NULL CHECK (role IN ('user', 'dispatcher')),
    expires_at timestamptz NOT NULL
);

CREATE TABLE alerts (
    id text PRIMARY KEY,
    patient_id text NOT NULL REFERENCES profiles(id),
    client_request_id text NOT NULL,
    source text NOT NULL CHECK (source IN ('manual', 'simulation')),
    status text NOT NULL CHECK (status IN ('awaiting_response', 'received', 'acknowledged', 'cancelled', 'closed')),
    profile_snapshot jsonb NOT NULL,
    location jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deadline_at timestamptz,
    received_at timestamptz,
    acknowledged_at timestamptz,
    closed_at timestamptz,
    UNIQUE (patient_id, client_request_id)
);

CREATE UNIQUE INDEX one_active_alert_per_patient ON alerts(patient_id)
    WHERE status IN ('awaiting_response', 'received', 'acknowledged');
CREATE INDEX alerts_created_at ON alerts(created_at DESC);

CREATE TABLE alert_requests (
    patient_id text NOT NULL REFERENCES profiles(id),
    client_request_id text NOT NULL,
    alert_id text NOT NULL REFERENCES alerts(id),
    PRIMARY KEY (patient_id, client_request_id)
);

CREATE TABLE alert_events (
    id bigserial PRIMARY KEY,
    alert_id text NOT NULL REFERENCES alerts(id),
    patient_id text NOT NULL REFERENCES profiles(id),
    kind text NOT NULL,
    status text NOT NULL,
    actor text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX alert_events_alert ON alert_events(alert_id, id);

-- +goose Down
DROP TABLE alert_events;
DROP TABLE alert_requests;
DROP TABLE alerts;
DROP TABLE sessions;
DROP TABLE profiles;
