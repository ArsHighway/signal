export type Role = 'user' | 'dispatcher';
export type Status = 'awaiting_response' | 'received' | 'acknowledged' | 'cancelled' | 'closed';
export interface Profile {
  id: string; full_name: string; birth_date: string; address: string;
  apartment: string; entrance: string; floor: string; access_notes: string;
  contact_name: string; contact_phone: string; conditions: string; allergies: string; updated_at: string;
}
export interface GeoLocation { latitude: number; longitude: number; accuracy: number; captured_at: string }
export interface AlertEvent { id: number; alert_id: string; kind: string; status: Status; actor: string; created_at: string }
export interface Alert {
  id: string; source: 'manual' | 'simulation'; status: Status; profile: Profile; location: GeoLocation | null;
  created_at: string; updated_at: string; deadline_at: string | null; received_at: string | null;
  acknowledged_at: string | null; closed_at: string | null; events: AlertEvent[];
}
export interface Dashboard { profile?: Profile; alerts: Alert[]; cursor: number; server_time: string; mode: 'training' }
export interface CreateRequest { source: 'manual' | 'simulation'; client_request_id: string; wait_seconds: number; location: GeoLocation | null }
